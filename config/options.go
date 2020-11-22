package config

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/fatih/color"
)

var usageStr = `
Usage: urlhunter [options]
Options:
    -d, --date <date>        A single date or a range to search.
                             Single: YYYY-MM-DD
                             Range:YYYY-MM-DD:YYYY-MM-DD
    -k, --keywords <file>    A txt file that contains strings to search.
    -o, --output <file>	     Output file
    -h, --help               Show this message
`

// PrintUsageErrorAndDie ...
func PrintUsageErrorAndDie(err error) {
	color.Red(err.Error())
	fmt.Println(usageStr)
	os.Exit(1)
}

// PrintHelpAndDie ...
func PrintHelpAndDie() {
	fmt.Println(usageStr)
	os.Exit(0)
}

// Options is main value holder agentgo-server flags.
type Options struct {
	Date     string `json:"date"`
	Keywords string `json:"keywords"`
	Output   string `json:"output"`
	ShowHelp bool   `json:"show_help"`
}

// ConfigureOptions accepts a flag set and augments it with agentgo-server
// specific flags. On success, an options structure is returned configured
// based on the selected flags.
func ConfigureOptions(fs *flag.FlagSet, args []string) (*Options, error) {

	// Create empty options
	opts := &Options{}

	// Define flags
	fs.BoolVar(&opts.ShowHelp, "h", false, "Show help message")
	fs.BoolVar(&opts.ShowHelp, "help", false, "Show help message")
	fs.StringVar(&opts.Date, "d", "", "A single date or a range to search. Single: YYYY-MM-DD Range:YYYY-MM-DD:YYYY-MM-DD")
	fs.StringVar(&opts.Date, "date", "", "A single date or a range to search. Single: YYYY-MM-DD Range:YYYY-MM-DD:YYYY-MM-DD")
	fs.StringVar(&opts.Keywords, "k", "", "A txt file that contains strings to search.")
	fs.StringVar(&opts.Keywords, "keywords", "", "A txt file that contains strings to search.")
	fs.StringVar(&opts.Output, "o", "", "Output file")
	fs.StringVar(&opts.Output, "output", "", "Output file")

	// Parse arguments and check for errors
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// If it is not help and other args are empty, return error
	if (opts.ShowHelp == false) && (opts.Date == "" || opts.Keywords == "" || opts.Output == "") {
		err := errors.New("please specify all arguments")
		return nil, err
	}

	return opts, nil
}
