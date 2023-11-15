```
				o  	  Utku Sen's
				 \_/\o   
				( Oo)                    \|/
				(_=-)  .===O-  ~~U~R~L~~ -O-
				/   \_/U'        hunter  /|\
				||  |_/
				\\  |    utkusen.com
				{K ||	twitter.com/utkusen
```

urlhunter is a recon tool that allows searching on URLs that are exposed via shortener services such as bit.ly and goo.gl. The project is written in Go.

### How?

A group named [URLTeam](https://archiveteam.org/index.php?title=URLTeam) (kudos to them) are brute forcing the URL shortener services and publishing matched results on a daily basis. urlhunter downloads their [collections](https://archive.org/details/UrlteamWebCrawls) and lets you analyze them. 

# Installation

## From Binary

You can download the pre-built binaries from the [releases](https://github.com/utkusen/urlhunter/releases/latest) page and run. For example:

`tar xzvf urlhunter_0.1.0_Linux_amd64.tar.gz`

`./urlhunter --help`

## From Source

1) Install Go on your system

2) Run: `go install github.com/utkusen/urlhunter@latest`

**Note For The Windows Users:** urlhunter uses `XZ Utils` which is pre-installed on Linux and macOS systems. For Windows systems, you need to download it from [https://tukaani.org/xz/](https://tukaani.org/xz/)

# Usage

urlhunter requires 3 parameters to run: `-keywords`, `-date` and `-o`. 

For example: `urlhunter --keywords keywords.txt --date 2020-11-20 --o out.txt`

### --keywords

You need to specify the txt file that contains keywords to search on URLs. Keywords must be written line by line. You have three different ways to specify keywords:

**Single Keyword:** urlhunter will search the given keyword as a substring. For example:

`acme.com` keyword will both match `https://acme.com/blabla` and `https://another.com/?referrer=acme.com`

**Multiple Keywords:** urlhunter will search the given keywords with an `AND` logic. Which means, a URL must include all the provided keywords. Keywords must be separated with `,` character. For example:

`acme.com,admin` will match `https://acme.com/secret/adminpanel` but **_won't_** match `https://acme.com/somethingelse`

**Regex Values:** urlhunter will search for the given regex value. In the keyword file, the line that contains a regular expression formula must start with `regex ` string. The format is: `regex REGEXFORMULA`. For example:

`regex 1\d{10}` will match `https://example.com/index.php?id=12938454312` but **_won't_** match `https://example.com/index.php?id=abc223`

### --date

urlhunter downloads the archive files of the given date(s). You have three different ways to specify the date:

**Latest:** urlhunter will download the latest archive. `-date latest`

**Single Date:** urlhunter will download the archive of the given date. Date format is YYYY-MM-DD. 

For example: `-date 2020-11-20`

**Date Range:** urlhunter will download all the archives between given start and end dates. 

For example: `-date 2020-11-10:2020-11-20`

### --output

You can specify the output file with `-o` parameter. For example `-o out.txt`

### Demonstration Video

[![Watch the video](https://i.imgur.com/J2CrvfM.png)](https://www.youtube.com/watch?v=Ct086YRm7i8)

## The Speed Problem

Archive.org throttles the speed when downloading files. Therefore, downloading an archive takes more time than usual. As a workaround, you can download the archives via Torrent and put them under the `archive/` folder which is located in the same directory with the urlhunter's binary. The directory tree will look like:

```
|-urlhunter
|---urlhunter(binary)
|---archive
|-----urlteam_2020-11-20-11-17-04
|-----urlteam_2020-11-17-11-17-04
```

## Example Use Cases

urlhunter might be useful for cyber intelligence and bug bounty purposes. For example:

`docs.google.com/a/acme.com` `drive.google.com/a/acme.com` keywords allow you to find public Google Docs&Drive share links of Acme company.

`acme.com,password_reset_token` keyword may allow you to find the working password reset tokens of acme.com

`trello.com` allows you to find public Trello addresses.

## Thanks

Special thanks to Samet Bekmezci([@sametbekmezci](https://twitter.com/sametbekmezci)) who gave me the idea of this tool. 

# Donation

Loved the project? You can buy me a coffee

<a href="https://www.buymeacoffee.com/utkusen" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/default-orange.png" alt="Buy Me A Coffee" height="41" width="174"></a>
