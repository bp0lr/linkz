//
// @bp0lr - 30/11/2020
//

package main

import (
	"path"
	"os"
	"fmt"
	"sync"
	"bufio"
	"regexp"
	"strings"
	"net/url"
	
	files	"github.com/bp0lr/linkz/fileutils"
	web		"github.com/bp0lr/linkz/fetch"
	filter	"github.com/bp0lr/linkz/static"

	flag 	"github.com/spf13/pflag"
	bs 		"github.com/pysrc/bs"
	random 	"github.com/thanhpk/randstr"
	tld 	"github.com/weppos/publicsuffix-go/publicsuffix"
)

var (
		workersArg				int
		timeOutArg				int
		urlArg					string
		outputFileArg			string
		outputFolderArg			string
		proxyArg				string
		headerArg         		[]string
		verboseArg				bool
		pbArg					bool
		followRedirectArg		bool
		saveInlineArg			bool
		downloadArg				bool
)

func main() {

	flag.IntVarP(&workersArg, "workers", "w", 25, "Number of workers")
	flag.StringVarP(&urlArg, "url", "u", "", "Target URL")
	flag.BoolVarP(&verboseArg, "verbose", "v", false, "Add verboicity to the process")
	flag.BoolVarP(&saveInlineArg, "save-inline", "s", false, "Save Inline javascript blocks")
	flag.BoolVarP(&downloadArg, "download", "d", false, "download files")
	flag.StringVarP(&outputFileArg, "output", "o", "", "Output file to save the results to")
	flag.StringVarP(&outputFolderArg, "folder", "f", "", "Output files to this folder")
	flag.IntVar(&timeOutArg, "timeout", 5, "Request timeOut in second")
	flag.BoolVar(&pbArg, "use-pb", false, "use a progress bar")
	flag.StringVarP(&proxyArg, "proxy", "p", "", "Use HTTP proxy")
	flag.StringArrayVarP(&headerArg, "header", "H", nil, "Add HTTP headers")
	flag.BoolVar(&followRedirectArg, "follow-redirect", false, "Follow redirects (Default: false)")

	flag.Parse()

	if (len(outputFolderArg) > 0){
		downloadArg = true
	}

	//concurrency
	workers := 25
	if workersArg > 0  &&  workersArg < 151 {
		workers = workersArg
	}else{
		fmt.Printf("[+] Workers amount should be between 1 and 150.\n")
		fmt.Printf("[+] The number of workers was set to 25.\n")		
	}
	
	if(verboseArg){
		fmt.Printf("[+] Workers: %v\n", workers)
	}

	var outputFile *os.File
	var err0 error
	if outputFileArg != "" {
		outputFile, err0 = os.OpenFile(outputFileArg, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
		if err0 != nil {
			fmt.Printf("cannot write %s: %s", outputFileArg, err0.Error())
			return
		}
		
		defer outputFile.Close()
	}

	var jobs []string

	if len(urlArg) < 1 {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			jobs = append(jobs, sc.Text())
		}
	} else {
		jobs = append(jobs, urlArg)
	}

	///////////////////////////////
	// Code processing
	///////////////////////////////

	conf:=web.HTTPConf{Timeout: timeOutArg, Proxy: proxyArg, Redirect: followRedirectArg, Headers: headerArg}

	var linksToDownload []string

	targetDomains := make(chan string)
	var wg sync.WaitGroup
	var mu = &sync.Mutex{}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			for task := range targetDomains {				
				source, err:=web.Get(task, headerArg, conf)
				if(err == nil){
					links:=getLinks(task, source)
					if(len(links) > 0){
						mu.Lock()
						linksToDownload=append(linksToDownload, links...)
						mu.Unlock()
					}

					var output string
					p, err := url.Parse(task)
					if(err == nil){
						output = path.Join(outputFolderArg, p.Host)
					}else{
						output = outputFolderArg
					}

					if(saveInlineArg){
						if(len(outputFolderArg) > 0){
							saveInline(source, output)
						} else {
							fmt.Printf("Please specify the download folder using -f")
							os.Exit(0)
						}						
					}
				}else{
					if(verboseArg){
						fmt.Printf("[-] %v => %v\n", task, err)
					}
				}				
			}
			wg.Done()
		}()
	}
		
	for _, line := range jobs {
		targetDomains <- line
	}
	
	close(targetDomains)	
	wg.Wait()	

	if(downloadArg && len(outputFolderArg) > 0){
		if(len(linksToDownload) > 0){
			downloadList(workers, conf, linksToDownload)
		}else{
			if(verboseArg){
				fmt.Printf("Nothing to download. The links list is empty")
				os.Exit(0)	
			}
		}
	} else {
		fmt.Printf("Please specify the download folder using -f")
		os.Exit(0)
	}		

	for _,v:=range linksToDownload{
		fmt.Printf("%v\n", v)
	}
}

func saveInline(source []byte, output string){
	soup := bs.Init(string(source))
	for _, j := range soup.SelByTag("script") {
		if(len(j.Value) > 0){
			fullPath:= path.Join(output, "inline_"+random.String(16) + ".txt")
			err:=files.CreateAndSaveToFile(fullPath, []byte(j.Value))
			if(err != nil){
				if(verboseArg){
					fmt.Printf("I can't save the inline script: %v\n", err)
				}
			}
		}
	}
}

func getLinks(webPage string, res []byte) []string{

	var urlParsingPattern = `(?:"|')(((?:[a-zA-Z]{1,10}://|//)[^"'/]{1,}\.[a-zA-Z]{2,}[^"']{0,})|((?:/|\.\./|\./)[^"'><,;| *()(%%$^/\\\[\]][^"'><,;|()]{1,})|([a-zA-Z0-9_\-/]{1,}/[a-zA-Z0-9_\-/]{1,}\.(?:[a-zA-Z]{1,4}|action)(?:[\?|#][^"|']{0,}|))|([a-zA-Z0-9_\-/]{1,}/[a-zA-Z0-9_\-/]{3,}(?:[\?|#][^"|']{0,}|))|([a-zA-Z0-9_\-]{1,}\.(?:php|asp|aspx|jsp|json|action|html|js|txt|xml)(?:[\?|#][^"|']{0,}|)))(?:"|')`
	urlParsingRegex, _ := regexp.Compile(urlParsingPattern)

	ignoredFileTypesPattern := `\.js|\.js\?`
	ignoredFileTypesRegex := regexp.MustCompile(ignoredFileTypesPattern)

	regexLinks := urlParsingRegex.FindAll(res, -1)

	totalLinks:=len(regexLinks)

	var validate bool = true
	domParse, err:=tld.Parse(webPage)
	if(err != nil){
		if(verboseArg){
			fmt.Printf("DOMPARSE ERROR: %v\n", err)
		}
		validate = false
	}
			
	var result []string
	for _, link := range regexLinks {
		
		var addLink bool = false

		u := string(link)
		
		// Skip blank entries
		if len(u) <= 0 {
			continue
		}
		
		// Remove the single and double quotes from the parsed link on the ends
		u = strings.Trim(u, "\"")
		u = strings.Trim(u, "'")
		
		//local path => full url
		p, err := url.Parse(u)
		if err != nil || p.Scheme == "" || p.Host == "" || p.Path == "" {
			u=completeUrls(u, webPage)
			p, err = url.Parse(u)
			if err != nil || p.Scheme == "" || p.Host == "" || p.Path == "" {
				continue
			}
		}

		matchString := ignoredFileTypesRegex.MatchString(u)
		if matchString {

			//matching domain name
			if(validate){
				if(strings.Contains(u, domParse.SLD)){
					addLink = true
					//fmt.Printf("link domain ok: %v\n", u)
				}else{
					//fmt.Printf("link err: %v\n", u)
				}
			}			
			
			//matching name agains our blacklist
			if(addLink && filter.Exist(files.GetFileNameFromLink(u))){
				addLink = false
			}else{
				//fmt.Printf("link BL OK: %v\n", u)
			}
									
			if(addLink){
				result = append(result, u)
			}			
		}
	}

	if(verboseArg){
		fmt.Printf("[%v] total: %v || valid: %v \n", webPage, totalLinks, len(result))
	}

	return result
}

func downloadList(workers int, conf web.HTTPConf, list []string){

	targetLinks := make(chan string)
	var wg sync.WaitGroup
	
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			for task := range targetLinks {
				source, err:=web.Get(task, headerArg, conf)
				if(err == nil){
					u, err := url.ParseRequestURI(task)
					if err != nil {
						if(verboseArg){
							fmt.Printf("[-] %v => %v\n", u, err)
							return	
						} 
					}

					_, fileName := path.Split(u.Path)
					output := path.Join(outputFolderArg, u.Host, fileName)					
					err=files.CreateAndSaveToFile(output, source)
					if(err != nil){
						if(verboseArg){
							fmt.Printf("I can't save the inline script: %v\n", err)
						} 
					}
				}				
			}
			wg.Done()
		}()
	}
		
	for _, line := range list {
		targetLinks <- line
	}
	
	close(targetLinks)
	wg.Wait()	
	
}

func completeUrls(link string, fullURL string)(string){

	var res string

	u, err := url.ParseRequestURI(fullURL)
	if(err != nil){
		return res
	}

	if strings.HasPrefix(link, "//") {
		res = u.Scheme + ":" + link
	} else if strings.HasPrefix(link, "/") && string(link[1]) != "/" {
		res = u.Scheme + "://" + u.Host + link
	} else if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		res = u.Scheme + "://" + u.Host + u.Path + "/" + link
	}
		
	return res
}