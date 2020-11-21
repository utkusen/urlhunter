package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
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

	if flag == false {
		color.Red("Couldn't find an archive with that date!")
		return
	}
	if ifArchiveExists(fullname) {
		color.Cyan(fullname + " Archive already exists!")
	} else {
		_ = os.Remove("archives/" + fullname + "/goo-gl/______.txt")
		_ = os.Remove("archives/" + fullname + "/bitly_6/______.txt")
		googfile := "goo-gl." + strings.Split(fullname, "_")[1] + ".zip"
		bitfile := "bitly_6." + strings.Split(fullname, "_")[1] + ".zip"
		if fileExists("archives/"+fullname+"/"+googfile) == false {
			color.Red(googfile + " doesn't exists locally.")
			url1 := "https://archive.org/download/" + fullname + "/" + googfile
			downloadFile(url1)
		}
		if fileExists("archives/"+fullname+"/"+bitfile) == false {
			color.Red(bitfile + " doesn't exists locally.")
			url2 := "https://archive.org/download/" + fullname + "/" + bitfile
			downloadFile(url2)
		}
		color.Magenta("Unzipping: " + googfile)
		_, err := Unzip("archives/"+fullname+"/"+googfile, "archives/"+fullname)
		if err != nil {
			color.Red(googfile + " looks damaged. It's removed now. Run the program again to re-download.")
			os.Remove("archives/" + fullname + "/" + googfile)
			os.Exit(1)
		}
		color.Magenta("Unzipping: " + bitfile)
		_, err = Unzip("archives/"+fullname+"/"+bitfile, "archives/"+fullname)
		if err != nil {
			color.Red(bitfile + " looks damaged. It's removed. Run the program again.")
			os.Remove("archives/" + fullname + "/" + bitfile)
			os.Exit(1)
		}
		color.Cyan("Decompressing XZ Archives..")
		_, err = exec.Command("xz", "--decompress", "archives/"+fullname+"/goo-gl/______.txt.xz").Output()
		if err != nil {
			panic(err)
		}
		_, err = exec.Command("xz", "--decompress", "archives/"+fullname+"/bitly_6/______.txt.xz").Output()
		if err != nil {
			panic(err)
		}
		color.Cyan("Removing Zip Files..")
		_ = os.Remove("archives/" + fullname + "/" + googfile)
		_ = os.Remove("archives/" + fullname + "/" + bitfile)
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
		searchFile("archives/"+fullname+"/goo-gl/______.txt", keywordSlice[i], outfile)
		searchFile("archives/"+fullname+"/bitly_6/______.txt", keywordSlice[i], outfile)
	}

}

func searchFile(fileLocation string, keyword string, outfile string) {
	path := strings.Split(fileLocation, "/")[1] + "/" + strings.Split(fileLocation, "/")[2]
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
				if foundFlag == true {
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
	googtxt := "archives/" + fullname + "/goo-gl/______.txt"
	bittxt := "archives/" + fullname + "/bitly_6/______.txt"
	googflag := fileExists(googtxt)
	bitflag := fileExists(bittxt)
	if googflag == false || bitflag == false {
		return false
	} else {
		return true
	}
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
	_ = os.MkdirAll("archives/"+dirname, os.ModePerm)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	f, err := os.OpenFile("archives/"+dirname+"/"+filename, os.O_CREATE|os.O_WRONLY, 0644)
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
