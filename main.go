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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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

const usage = `Usage: ./urlhunter --keywords /path/to/keywordsFile --date DATE-RANGE-HERE [--output /path/to/outputFile] [--archives /path/to/archives] [--rm]
Example: ./urlhunter --keywords keywords.txt --date 2020-11-20
  -k, --keywords /path/to/keywordsFile
      Path to a file that contains strings to search.

  -d, --date DATE-RANGE-HERE
      You may specify either a single date, a range, or a year:
      Single date: "2020-11-20"
      Date range: "2020-11-10:2020-11-20"
      Full year: "2024" (will search the entire year)

  -o, --output /path/to/outputFile
      (Optional) Path to a file where the output will be written. If not specified, results will be printed to screen.

  -a, --archives /path/to/archives
      (Optional) Path to the directory where you're storing your archive files. If this is your first time running this tool, the archives will be downloaded on a new ./archives folder

  --rm
      (Optional) Remove downloaded archive folders after processing to save disk space.
`

var err error
var archivesPath string

// BeaconMetadata stores metadata from the header of a Beacon file
type BeaconMetadata struct {
	prefix     string
	target     string
	relation   string
	message    string
	annotation string
}

// BeaconLine represents a single line in a Beacon file
type BeaconLine struct {
	Source string
	Target string
}

// ReadMetadata reads and parses the metadata section of a beacon file
func ReadMetadata(filename string) (BeaconMetadata, error) {
	f, err := os.Open(filename)
	m := BeaconMetadata{}
	if err != nil {
		return m, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "#") {
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		value := strings.TrimSpace(parts[1])

		switch parts[0] {
		case "#PREFIX":
			m.prefix = value
		case "#TARGET":
			m.target = value
		case "#RELATION":
			m.relation = value
		case "#MESSAGE":
			m.message = value
		case "#ANNOTATION":
			m.annotation = value
		}
	}
	return m, nil
}

// ParseLine parses a single line from a beacon file
func ParseLine(line string, data BeaconMetadata) (BeaconLine, error) {
	b := BeaconLine{}

	// First, try to find the first occurrence of | that's not part of a URL
	var parts []string
	lastIndex := 0
	inURL := false

	for i := 0; i < len(line); i++ {
		if i+7 <= len(line) && (strings.HasPrefix(line[i:], "http://") || strings.HasPrefix(line[i:], "https://")) {
			inURL = true
			continue
		}

		if line[i] == ' ' && inURL {
			// Space usually indicates end of URL
			inURL = false
		}

		if line[i] == '|' && !inURL {
			parts = append(parts, strings.TrimSpace(line[lastIndex:i]))
			lastIndex = i + 1
		}
	}
	parts = append(parts, strings.TrimSpace(line[lastIndex:]))

	if len(parts) > 3 {
		// If we still have more than 3 parts after our smarter parsing,
		// it's truly an invalid line
		return b, fmt.Errorf("invalid line: too many parts")
	}

	// Handle the source URL
	source := parts[0]
	if data.prefix != "" {
		if isURL(data.prefix) {
			source = joinURLs(data.prefix, source)
		} else {
			source = joinURLs(source, data.prefix)
		}
	}
	b.Source = source

	// Handle the target URL
	var target string
	if len(parts) == 1 {
		target = parts[0]
	} else if len(parts) == 2 && isURL(parts[1]) {
		target = parts[1]
	} else if len(parts) == 3 {
		target = parts[2]
	} else {
		target = parts[0]
	}

	if data.target != "" {
		if isURL(data.target) {
			target = joinURLs(data.target, target)
		} else {
			target = joinURLs(target, data.target)
		}
	}
	b.Target = target

	return b, nil
}

// isURL checks if a string is a valid URL
func isURL(s string) bool {
	_, err := url.ParseRequestURI(s)
	return err == nil
}

// joinURLs joins two URL parts together
func joinURLs(base, path string) string {
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base + strings.TrimPrefix(path, "/")
}

type ArchiveJob struct {
	date        string
	keywords    string
	outfile     string
	removeAfter bool
}

type ProcessSummary struct {
	Date   string
	Status string
	Error  error
}

func processArchives(jobs []ArchiveJob) {
	maxWorkers := 3
	var wg sync.WaitGroup
	jobChan := make(chan ArchiveJob, len(jobs))
	summaryChan := make(chan ProcessSummary, len(jobs))

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				archiveList := getArchiveList()
				err := getArchive(archiveList, job.date, job.keywords, job.outfile, job.removeAfter)
				if err != nil {
					summaryChan <- ProcessSummary{
						Date:   job.date,
						Status: "Failed",
						Error:  err,
					}
				} else {
					summaryChan <- ProcessSummary{
						Date:   job.date,
						Status: "Success",
					}
				}
			}
		}()
	}

	// Send jobs to workers
	for _, job := range jobs {
		jobChan <- job
	}
	close(jobChan)

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(summaryChan)
	}()

	// Collect and display summary
	var failed []ProcessSummary
	var succeeded []string

	for summary := range summaryChan {
		if summary.Error != nil {
			failed = append(failed, summary)
			color.Red("[FAILED] Date %s: %v", summary.Date, summary.Error)
		} else {
			succeeded = append(succeeded, summary.Date)
			color.Green("[SUCCESS] Date %s processed successfully", summary.Date)
		}
	}

	// Print final summary
	fmt.Println("\n=== Final Summary ===")
	color.Green("Successfully processed dates: %d", len(succeeded))
	if len(failed) > 0 {
		color.Red("Failed dates: %d", len(failed))
		color.Yellow("\nFailed dates details:")
		for _, f := range failed {
			color.Yellow("- %s: %v", f.Date, f.Error)
		}
	}
}

func main() {
	var keywordFile string
	var dateParam string
	var outFile string
	var removeAfter bool

	flag.StringVar(&keywordFile, "k", "", "A txt file that contains strings to search.")
	flag.StringVar(&keywordFile, "keywords", "", "A txt file that contains strings to search.")
	flag.StringVar(&dateParam, "d", "", "A single date, range, or year. Single: YYYY-MM-DD Range: YYYY-MM-DD:YYYY-MM-DD Year: YYYY")
	flag.StringVar(&dateParam, "date", "", "A single date, range, or year. Single: YYYY-MM-DD Range: YYYY-MM-DD:YYYY-MM-DD Year: YYYY")
	flag.StringVar(&outFile, "o", "", "Output file")
	flag.StringVar(&outFile, "output", "", "Output file")
	flag.StringVar(&archivesPath, "a", "archives", "Archives file path")
	flag.StringVar(&archivesPath, "archives", "archives", "Archives file path")
	flag.BoolVar(&removeAfter, "rm", false, "Remove downloaded archive folders after processing")

	flag.Usage = func() { fmt.Print(usage) }
	flag.Parse()

	if keywordFile == "" || dateParam == "" {
		fmt.Println("Error: Missing required arguments")
		flag.Usage()
		os.Exit(1)
	}

	fmt.Println(`
	o  	  Utku Sen's
	 \_/\o
	( Oo)                    \|/
	(_=-)  .===O-  ~~U~R~L~~ -O-
	/   \_/U'        hunter  /|\
	||  |_/
	\\  |    utkusen.com
	{K ||	twitter.com/utkusen_en

 `)
	_ = os.Mkdir(archivesPath, os.ModePerm)

	// Handle year-only format
	if len(dateParam) == 4 {
		if _, err := time.Parse("2006", dateParam); err == nil {
			startDate := fmt.Sprintf("%s-01-01", dateParam)
			endDate := fmt.Sprintf("%s-12-31", dateParam)
			dateParam = fmt.Sprintf("%s:%s", startDate, endDate)
		}
	}

	if strings.Contains(dateParam, ":") {
		startDate, err := time.Parse("2006-01-02", strings.Split(dateParam, ":")[0])
		if err != nil {
			crash("Wrong date format!", err)
		}
		endDate, err := time.Parse("2006-01-02", strings.Split(dateParam, ":")[1])
		if err != nil {
			crash("Wrong date format!", err)
		}

		// Collect all dates into jobs
		var jobs []ArchiveJob
		for rd := rangeDate(startDate, endDate); ; {
			date := rd()
			if date.IsZero() {
				break
			}
			jobs = append(jobs, ArchiveJob{
				date:        date.Format("2006-01-02"),
				keywords:    keywordFile,
				outfile:     outFile,
				removeAfter: removeAfter,
			})
		}

		// Process all jobs in parallel
		processArchives(jobs)
	} else {
		if dateParam != "latest" {
			_, err := time.Parse("2006-01-02", dateParam)
			if err != nil {
				crash("Wrong date format!", err)
			}
		}

		// Single date, just process it directly
		archiveList := getArchiveList()
		err := getArchive(archiveList, dateParam, keywordFile, outFile, removeAfter)
		if err != nil {
			crash("Error processing archive", err)
		}
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

type DownloadJob struct {
	url      string
	dirname  string
	filename string
	item     Files
}

type ProcessResult struct {
	Error     error
	Dirname   string
	DumpPaths []string
}

func downloadAndProcess(dumpFiles Files, fullname string) ([]ProcessResult, error) {
	var downloads []DownloadJob
	results := make(chan ProcessResult, len(dumpFiles.File))

	// Collect all files that need to be downloaded
	for _, item := range dumpFiles.File {
		if !fileExists(filepath.Join(archivesPath, fullname, item.Name)) {
			info(item.Name + " doesn't exist locally. Will be downloaded.")
			url := "https://archive.org/download/" + fullname + "/" + item.Name
			downloads = append(downloads, DownloadJob{
				url:      url,
				dirname:  fullname,
				filename: item.Name,
				item:     dumpFiles,
			})
		} else {
			// If file exists, process it immediately
			go func(item struct{ Name, DumpType string }) {
				result := processArchive(fullname, item)
				results <- result
			}(struct{ Name, DumpType string }{Name: item.Name, DumpType: item.DumpType})
		}
	}

	if len(downloads) > 0 {
		var wg sync.WaitGroup

		// Start parallel downloads and process each file as it completes
		for _, job := range downloads {
			wg.Add(1)
			go func(job DownloadJob) {
				defer wg.Done()

				// Download the file
				if err := downloadFile(job.url); err != nil {
					results <- ProcessResult{
						Error: fmt.Errorf("failed to download %s: %v", job.filename, err),
					}
					return
				}

				// Process it immediately after download
				result := processArchive(job.dirname, struct{ Name, DumpType string }{
					Name:     job.filename,
					DumpType: strings.Split(job.filename, ".")[0],
				})
				results <- result
			}(job)
		}

		// Wait for all downloads and processing to complete
		go func() {
			wg.Wait()
			close(results)
		}()
	} else {
		close(results)
	}

	// Collect all results
	var processResults []ProcessResult
	for result := range results {
		if result.Error != nil {
			return nil, result.Error
		}
		processResults = append(processResults, result)
	}

	return processResults, nil
}

func processArchive(fullname string, item struct{ Name, DumpType string }) ProcessResult {
	result := ProcessResult{
		Dirname: fullname,
	}

	// Remove any existing txt files
	dumpFilepath, _ := filepath.Glob(filepath.Join(archivesPath, fullname, item.DumpType, "*.txt"))
	if len(dumpFilepath) > 0 {
		_ = os.Remove(dumpFilepath[0])
	}

	// Unzip the file
	info("Unzipping: " + item.Name)
	_, err := Unzip(filepath.Join(archivesPath, fullname, item.Name), filepath.Join(archivesPath, fullname))
	if err != nil {
		os.Remove(filepath.Join(archivesPath, fullname, item.Name))
		result.Error = fmt.Errorf("failed to unzip %s: %v", item.Name, err)
		return result
	}

	// Decompress XZ archive
	info("Decompressing: " + item.Name)
	tarfile, _ := filepath.Glob(filepath.Join(archivesPath, fullname, item.DumpType, "*.txt.xz"))
	if len(tarfile) > 0 {
		_, err := exec.Command("xz", "--decompress", tarfile[0]).Output()
		if err != nil {
			result.Error = fmt.Errorf("failed to decompress %s: %v", item.Name, err)
			return result
		}
	}

	// Get the path of the decompressed file
	dumpPath, _ := filepath.Glob(filepath.Join(archivesPath, fullname, item.DumpType, "*.txt"))
	if len(dumpPath) > 0 {
		result.DumpPaths = dumpPath
	}

	// Remove the zip file
	_ = os.Remove(filepath.Join(archivesPath, fullname, item.Name))

	return result
}

func getArchive(body []byte, date string, keywordFile string, outfile string, removeAfter bool) error {
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
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}

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
		return fmt.Errorf("no archive found for date %s", date)
	}

	dumpFiles := archiveMetadata(fullname)
	if ifArchiveExists(fullname) {
		info(fullname + " already exists locally. Skipping download..")
	} else {
		// Download and process files in parallel
		results, err := downloadAndProcess(dumpFiles, fullname)
		if err != nil {
			return fmt.Errorf("processing failed: %v", err)
		}

		// Process keywords for each successfully processed file
		fileBytes, err := ioutil.ReadFile(keywordFile)
		if err != nil {
			return fmt.Errorf("failed to read keyword file: %v", err)
		}

		keywordSlice := strings.Split(string(fileBytes), "\n")
		for i := 0; i < len(keywordSlice); i++ {
			if keywordSlice[i] == "" {
				continue
			}
			for _, result := range results {
				for _, dumpPath := range result.DumpPaths {
					if err := searchFile(dumpPath, keywordSlice[i], outfile); err != nil {
						return fmt.Errorf("search failed in %s: %v", dumpPath, err)
					}
				}
			}
		}
	}

	// Remove archive folder if --rm is specified
	if removeAfter {
		archiveDir := filepath.Join(archivesPath, fullname)
		info("Removing archive folder: " + archiveDir)
		if err := os.RemoveAll(archiveDir); err != nil {
			warning("Failed to remove archive folder: " + err.Error())
		}
	}

	return nil
}

func searchFile(fileLocation string, keyword string, outfile string) error {
	var path string

	if strings.HasPrefix(fileLocation, "archives") {
		path_parts := strings.Split(fileLocation, string(os.PathSeparator))
		path = filepath.Join(path_parts[1], path_parts[2])
	} else {
		path = fileLocation
	}

	info("Searching: \"" + keyword + "\" in " + path)

	f, err := os.Open(fileLocation)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var outputFile *os.File
	if outfile != "" {
		outputFile, err = os.OpenFile(outfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer outputFile.Close()
	}

	metadata, err := ReadMetadata(fileLocation)
	if err != nil {
		warning(err.Error())
		return err
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
		return err
	}

	for scanner.Scan() {
		if matcher(scanner.Bytes()) {
			line, err := ParseLine(scanner.Text(), metadata)
			if err != nil {
				warning(err.Error())
				continue
			}
			textToWrite := fmt.Sprintf("%s,%s\n", line.Source, line.Target)

			if outfile != "" {
				if _, err := outputFile.WriteString(textToWrite); err != nil {
					return err
				}
			} else {
				fmt.Print(textToWrite)
			}
		}
	}
	return scanner.Err()
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

func downloadFile(url string) error {
	dirname := strings.Split(url, "/")[4]
	filename := strings.Split(url, "/")[5]
	info("Downloading: " + url)
	_ = os.MkdirAll(filepath.Join(archivesPath, dirname), os.ModePerm)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.OpenFile(filepath.Join(archivesPath, dirname, filename), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	bar := progressbar.DefaultBytes(
		resp.ContentLength,
		"",
	)
	_, err = io.Copy(io.MultiWriter(f, bar), resp.Body)
	if err != nil {
		return err
	}
	color.Green("Download Finished!")
	return nil
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
