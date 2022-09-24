package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/rzhade3/beaconspec"
	"github.com/schollz/progressbar/v3"
)

var baseurl string = "https://archive.org/services/search/v1/scrape?debug=false&xvar=production&total_only=false&count=10000&fields=identifier%2Citem_size&q=Urlteam%20Release"

type Files struct {
	XMLName xml.Name `xml:"files"`
	Text    string   `xml:",chardata"`
	File    []struct {
		Text     string `xml:",chardata"`
		Name     string `xml:"name,attr"`
		Source   string `xml:"source,attr"`
		Mtime    string `xml:"mtime"`
		Size     string `xml:"size"`
		Md5      string `xml:"md5"`
		Crc32    string `xml:"crc32"`
		Sha1     string `xml:"sha1"`
		Format   string `xml:"format"`
		Btih     string `xml:"btih"`
		DumpType string `xml:name,attr`
	} `xml:"file"`
}

const usage = `Usage: ./urlhunter --keywords /path/to/keywordsFile --date DATE-RANGE-HERE --output /path/to/outputFile [--archives /path/to/archives]
Example: ./urlhunter --keywords keywords.txt --date 2020-11-20 --output out.txt
  -k, --keywords /path/to/keywordsFile
      Path to a file that contains strings to search.

  -d, --date DATE-RANGE-HERE
      You may specify either a single date, or a range; Using a single date will set the present as the end of the range. 
      Single date: "2020-11-20". Range: "2020-11-10:2020-11-20".

  -o, --output /path/to/outputFile
      Path to a file where the output will be written.

  -a, --archives /path/to/archives
      Path to the directory where you're storing your archive files. If this is your first time running this tool, the archives will be downloaded on a new ./archives folder
`

var err error
var archivesPath string

func main() {
	var keywordFile string
	var dateParam string
	var outFile string

	flag.StringVar(&keywordFile, "k", "", "A txt file that contains strings to search.")
	flag.StringVar(&keywordFile, "keywords", "", "A txt file that contains strings to search.")
	flag.StringVar(&dateParam, "d", "", "A single date or a range to search. Single: YYYY-MM-DD Range:YYYY-MM-DD:YYYY-MM-DD")
	flag.StringVar(&dateParam, "date", "", "A single date or a range to search. Single: YYYY-MM-DD Range:YYYY-MM-DD:YYYY-MM-DD")
	flag.StringVar(&outFile, "o", "", "Output file")
	flag.StringVar(&outFile, "output", "", "Output file")
	flag.StringVar(&archivesPath, "a", "archives", "Archives file path")
	flag.StringVar(&archivesPath, "archives", "archives", "Archives file path")

	//https://www.antoniojgutierrez.com/posts/2021-05-14-short-and-long-options-in-go-flags-pkg/
	flag.Usage = func() { fmt.Print(usage) }
	flag.Parse()

	if keywordFile == "" || dateParam == "" || outFile == "" {
		crash("You must specify all arguments.", err)
		return
	}

	fmt.Println(`
	o  	  Utku Sen's
	 \_/\o
	( Oo)                    \|/
	(_=-)  .===O-  ~~U~R~L~~ -O-
	/   \_/U'        hunter  /|\
	||  |_/
	\\  |    utkusen.com
	{K ||	twitter.com/utkusen

 `)
	_ = os.Mkdir(archivesPath, os.ModePerm)
	if strings.Contains(dateParam, ":") {
		startDate, err := time.Parse("2006-01-02", strings.Split(dateParam, ":")[0])
		if err != nil {
			crash("Wrong date format!", err)
		}
		endDate, err := time.Parse("2006-01-02", strings.Split(dateParam, ":")[1])
		if err != nil {
			crash("Wrong date format!", err)
		}
		for rd := rangeDate(startDate, endDate); ; {
			date := rd()
			if date.IsZero() {
				break
			}
			getArchive(getArchiveList(), string(date.Format("2006-01-02")), keywordFile, outFile)
		}
	} else {
		if dateParam != "latest" {
			_, err := time.Parse("2006-01-02", dateParam)
			if err != nil {
				crash("Wrong date format!", err)
			}
		}

		getArchive(getArchiveList(), dateParam, keywordFile, outFile)
	}
	color.Green("Search complete!")
}

func getArchiveList() []byte {
	resp, err := http.Get(baseurl)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	return body
}

func getArchive(body []byte, date string, keywordFile string, outfile string) {
	color.Cyan("Search starting for: " + date)
	type Response struct {
		Items []struct {
			Identifier string `json:"identifier"`
			ItemSize   int64  `json:"item_size"`
		} `json:"items"`
		Count int `json:"count"`
		Total int `json:"total"`
	}
	var response Response
	json.Unmarshal(body, &response)
	flag := false
	var fullname string
	if date == "latest" {
		fullname = response.Items[len(response.Items)-1].Identifier
		flag = true
	} else {
		for i := 0; i < len(response.Items); i++ {
			if strings.Contains(response.Items[i].Identifier, date) {
				fullname = response.Items[i].Identifier
				flag = true
				break
			}
		}
	}

	if !flag {
		info("Couldn't find an archive with that date.")
		return
	}
	dumpFiles := archiveMetadata(fullname)
	if ifArchiveExists(fullname) {
		info(fullname + " already exists locally. Skipping download..")
	} else {
		for _, item := range dumpFiles.File {
			dumpFilepath, _ := filepath.Glob(filepath.Join(archivesPath, fullname, item.DumpType, "*.txt"))
			if len(dumpFilepath) > 0 {
				_ = os.Remove(dumpFilepath[0])
			}

			if !fileExists(filepath.Join(archivesPath, fullname, item.Name)) {
				info(item.Name + " doesn't exist locally. The file will be downloaded.")
				url1 := "https://archive.org/download/" + fullname + "/" + item.Name
				downloadFile(url1)
			}

			info("Unzipping: " + item.Name)
			_, err := Unzip(filepath.Join(archivesPath, fullname, item.Name), filepath.Join(archivesPath, fullname))
			if err != nil {
				os.Remove(filepath.Join(archivesPath, fullname, item.Name))
				crash(item.Name+" looks damaged. It's removed now. Run the program again to re-download.", err)
			}
		}

		info("Decompressing XZ Archives..")
		for _, item := range dumpFiles.File {
			tarfile, _ := filepath.Glob(filepath.Join(archivesPath, fullname, item.DumpType, "*.txt.xz"))
			_, err := exec.Command("xz", "--decompress", tarfile[0]).Output()
			if err != nil {
				crash("Error decompressing the downloaded archives", err)
			}
		}

		info("Removing Zip Files..")
		for _, item := range dumpFiles.File {
			_ = os.Remove(filepath.Join(archivesPath, fullname, item.Name))
		}
	}
	fileBytes, err := ioutil.ReadFile(keywordFile)
	if err != nil {
		panic(err)
	}
	keywordSlice := strings.Split(string(fileBytes), "\n")
	for i := 0; i < len(keywordSlice); i++ {
		if keywordSlice[i] == "" {
			continue
		}
		for _, item := range dumpFiles.File {
			dump_path, _ := filepath.Glob(filepath.Join(archivesPath, fullname, item.DumpType, "*.txt"))
			searchFile(dump_path[0], keywordSlice[i], outfile)
		}
	}

}

func searchFile(fileLocation string, keyword string, outfile string) {
	var path string

	if strings.HasPrefix(fileLocation, "archives") {
		path_parts := strings.Split(fileLocation, string(os.PathSeparator))
		path = filepath.Join(path_parts[1], path_parts[2])
	} else {
		path = fileLocation
	}

	info("Searching: \"" + keyword + "\" in " + path)

	f, err := os.Open(fileLocation)
	scanner := bufio.NewScanner(f)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	f, err = os.OpenFile(outfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	metadata, err := beaconspec.ReadMetadata(fileLocation)
	if err != nil {
		warning(err.Error())
		return
	}

	var matcher func([]byte) bool
	if strings.HasPrefix(keyword, "regex") {
		matcher, err = regexMatch(keyword)
	} else if strings.Contains(keyword, ",") {
		matcher, err = multiKeywordMatcher(keyword)
	} else {
		matcher, err = stringMatch(keyword)
	}
	if err != nil {
		warning(err.Error())
		return
	}

	for scanner.Scan() {
		if matcher(scanner.Bytes()) {

			line, err := beaconspec.ParseLine(scanner.Text(), metadata)
			if err != nil {
				panic(err)
			}
			textToWrite := fmt.Sprintf("%s,%s\n", line.Source, line.Target)
			if _, err := f.WriteString(textToWrite); err != nil {
				panic(err)
			}
		}
	}
}

func regexMatch(keyword string) (func([]byte) bool, error) {
	regexValue := strings.Split(keyword, " ")[1]
	r, err := regexp.Compile(regexValue)
	return func(b []byte) bool {
		s := string(b)
		return r.MatchString(s)
	}, err
}

func multiKeywordMatcher(keyword string) (func([]byte) bool, error) {
	keywords := strings.Split(keyword, ",")
	bytes_keywords := make([][]byte, len(keywords))
	for i, k := range keywords {
		bytes_keywords[i] = []byte(k)
	}
	return func(text []byte) bool {
		for _, k := range bytes_keywords {
			if !bytes.Contains(text, k) {
				return false
			}
		}
		return true
	}, nil
}

func stringMatch(keyword string) (func([]byte) bool, error) {
	bytes_keyword := []byte(keyword)
	return func(b []byte) bool {
		return bytes.Contains(b, bytes_keyword)
	}, nil
}

func ifArchiveExists(fullname string) bool {
	dumpFiles := archiveMetadata(fullname)
	for _, item := range dumpFiles.File {
		archiveFilepaths, err := filepath.Glob(filepath.Join(archivesPath, fullname, item.DumpType, "*.txt"))
		if len(archiveFilepaths) == 0 || err != nil {
			return false
		}
	}
	return true
}

func archiveMetadata(fullname string) Files {
	metadataFilename := "urlteam_" + strings.Split(fullname, "_")[1] + "_files.xml"
	if !fileExists(filepath.Join(archivesPath, fullname, metadataFilename)) {
		info(metadataFilename + " doesn't exist locally. The file will be downloaded.")
		metadataUrl := "https://archive.org/download/" + fullname + "/" + metadataFilename
		downloadFile(metadataUrl)
	}
	byteValue, _ := ioutil.ReadFile(filepath.Join(archivesPath, fullname, metadataFilename))
	files := Files{}
	xml.Unmarshal(byteValue, &files)
	// Not all files are dumps, this struct will only contain zip dumps
	dumpFiles := Files{}
	for _, item := range files.File {
		if item.Format == "ZIP" {
			item.DumpType = strings.Split(item.Name, ".")[0]
			dumpFiles.File = append(dumpFiles.File, item)
		}
	}
	return dumpFiles
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func downloadFile(url string) {
	dirname := strings.Split(url, "/")[4]
	filename := strings.Split(url, "/")[5]
	info("Downloading: " + url)
	_ = os.MkdirAll(filepath.Join(archivesPath, dirname), os.ModePerm)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	f, err := os.OpenFile(filepath.Join(archivesPath, dirname, filename), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	bar := progressbar.DefaultBytes(
		resp.ContentLength,
		"",
	)
	io.Copy(io.MultiWriter(f, bar), resp.Body)
	color.Green("Download Finished!")
}

func Unzip(src string, dest string) ([]string, error) {
	var filenames []string
	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}
	defer r.Close()
	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s: illegal file path", fpath)
		}

		filenames = append(filenames, fpath)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return filenames, err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return filenames, err
		}
		rc, err := f.Open()
		if err != nil {
			return filenames, err
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return filenames, err
		}
	}
	return filenames, nil
}

func rangeDate(start, end time.Time) func() time.Time {
	y, m, d := start.Date()
	start = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	y, m, d = end.Date()
	end = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	return func() time.Time {
		if start.After(end) {
			return time.Time{}
		}
		date := start
		start = start.AddDate(0, 0, 1)
		return date
	}
}

func info(message string) {
	fmt.Println("[+]: " + message)
}

func crash(message string, err error) {
	color.Red("[ERROR]: " + message + "\n")
	panic(err)
}

func warning(message string) {
	color.Yellow("[WARNING]: " + message + "\n")
}
