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

func main() {
	keywordFile := flag.String("keywords", "", "A txt file that contains strings to search.")
	dateParam := flag.String("date", "", "A single date or a range to search. Single: YYYY-MM-DD Range:YYYY-MM-DD:YYYY-MM-DD")
	outFile := flag.String("o", "", "Output file")
	flag.Parse()
	if *keywordFile == "" || *dateParam == "" || *outFile == "" {
		color.Red("Please specify all arguments!")
		flag.PrintDefaults()
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
	_ = os.Mkdir("archives", os.ModePerm)
	if strings.Contains(*dateParam, ":") {
		startDate, err := time.Parse("2006-01-02", strings.Split(*dateParam, ":")[0])
		if err != nil {
			color.Red("Wrong date format!")
			return
		}
		endDate, err := time.Parse("2006-01-02", strings.Split(*dateParam, ":")[1])
		if err != nil {
			color.Red("Wrong date format!")
			return
		}
		for rd := rangeDate(startDate, endDate); ; {
			date := rd()
			if date.IsZero() {
				break
			}
			getArchive(getArchiveList(), string(date.Format("2006-01-02")), *keywordFile, *outFile)
		}
	} else {
		if *dateParam != "latest" {
			_, err := time.Parse("2006-01-02", *dateParam)
			if err != nil {
				color.Red("Wrong date format!")
				return
			}
		}

		getArchive(getArchiveList(), *dateParam, *keywordFile, *outFile)
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
	fmt.Println("Search starting for: " + date)
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
		color.Red("Couldn't find an archive with that date!")
		return
	}
	dumpFiles := archiveMetadata(fullname)
	if ifArchiveExists(fullname) {
		color.Cyan(fullname + " Archive already exists!")
	} else {
		for _, item := range dumpFiles.File {
			dumpFilepath, _ := filepath.Glob(filepath.Join("archives", fullname, item.DumpType, "*.txt"))
			if len(dumpFilepath) > 0 {
				_ = os.Remove(dumpFilepath[0])
			}

			if !fileExists(filepath.Join("archives", fullname, item.Name)) {
				color.Red(item.Name + " doesn't exist locally.")
				url1 := "https://archive.org/download/" + fullname + "/" + item.Name
				downloadFile(url1)
			}

			color.Magenta("Unzipping: " + item.Name)
			_, err := Unzip(filepath.Join("archives", fullname, item.Name), filepath.Join("archives", fullname))
			if err != nil {
				color.Red(item.Name + " looks damaged. It's removed now. Run the program again to re-download.")
				os.Remove(filepath.Join("archives", fullname, item.Name))
				os.Exit(1)
			}
		}

		color.Cyan("Decompressing XZ Archives..")
		for _, item := range dumpFiles.File {
			tarfile, _ := filepath.Glob(filepath.Join("archives", fullname, item.DumpType, "*.txt.xz"))
			_, err := exec.Command("xz", "--decompress", tarfile[0]).Output()
			if err != nil {
				fmt.Println(err)
				panic(err)
			}
		}

		color.Cyan("Removing Zip Files..")
		for _, item := range dumpFiles.File {
			_ = os.Remove(filepath.Join("archives", fullname, item.Name))
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
			dump_path, _ := filepath.Glob(filepath.Join("archives", fullname, item.DumpType, "*.txt"))
			searchFile(dump_path[0], keywordSlice[i], outfile)
		}
	}

}

func searchFile(fileLocation string, keyword string, outfile string) {
	path_parts := strings.Split(fileLocation, string(os.PathSeparator))
	path := filepath.Join(path_parts[1], path_parts[2])
	fmt.Println("Searching: " + keyword + " in: " + path)
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
	if strings.HasPrefix(keyword, "regex") {
		regexValue := strings.Split(keyword, " ")[1]
		r, err := regexp.Compile(regexValue)
		if err != nil {
			color.Red("Invalid Regex!")
			return
		}
		for scanner.Scan() {
			if r.MatchString(scanner.Text()) {
				textToWrite := strings.Split(scanner.Text(), "|")[1]
				if _, err := f.WriteString(textToWrite + "\n"); err != nil {
					panic(err)
				}
			}
		}
	} else {
		if strings.Contains(keyword, ",") {
			keywords := strings.Split(keyword, ",")
			for scanner.Scan() {
				foundFlag := true
				for i := 0; i < len(keywords); i++ {
					if bytes.Contains(scanner.Bytes(), []byte(keywords[i])) {
						continue
					} else {
						foundFlag = false
					}
				}
				if foundFlag {
					textToWrite := strings.Split(scanner.Text(), "|")[1]
					if _, err := f.WriteString(textToWrite + "\n"); err != nil {
						panic(err)
					}
				}
			}

		} else {
			toFind := []byte(keyword)
			for scanner.Scan() {
				if bytes.Contains(scanner.Bytes(), toFind) {
					textToWrite := strings.Split(scanner.Text(), "|")[1]
					if _, err := f.WriteString(textToWrite + "\n"); err != nil {
						panic(err)
					}
				}
			}
		}
	}

}

func ifArchiveExists(fullname string) bool {
	dumpFiles := archiveMetadata(fullname)
	for _, item := range dumpFiles.File {
		archiveFilepaths, err := filepath.Glob(filepath.Join("archives", fullname, item.DumpType, "*.txt"))
		if len(archiveFilepaths) == 0 || err != nil {
			return false
		}
	}
	return true
}

func archiveMetadata(fullname string) Files {
	metadataFilename := "urlteam_" + strings.Split(fullname, "_")[1] + "_files.xml"
	if !fileExists(filepath.Join("archives", fullname, metadataFilename)) {
		color.Red(metadataFilename + " doesn't exists locally.")
		metadataUrl := "https://archive.org/download/" + fullname + "/" + metadataFilename
		downloadFile(metadataUrl)
	}
	byteValue, _ := ioutil.ReadFile(filepath.Join("archives", fullname, metadataFilename))
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
	fmt.Println("Downloading: " + url)
	_ = os.MkdirAll(filepath.Join("archives", dirname), os.ModePerm)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	f, err := os.OpenFile(filepath.Join("archives", dirname, filename), os.O_CREATE|os.O_WRONLY, 0644)
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

func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
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
