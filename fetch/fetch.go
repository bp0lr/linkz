package fetcher

import (
	//"fmt"
	"io/ioutil"
	"strings"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"
	"math/rand"	
)

//HTTPConf desc
type HTTPConf struct{
	Timeout 	int
	Proxy		string
	Redirect 	bool
	Headers		[]string
}

func newClient(conf HTTPConf) *http.Client {

	tr := &http.Transport{
		MaxIdleConns:    30,
		IdleConnTimeout: time.Second,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout: time.Second * time.Duration(conf.Timeout),
		}).DialContext,
	}

	if conf.Proxy != "" {
		if p, err := url.Parse(conf.Proxy); err == nil {
			tr.Proxy = http.ProxyURL(p)
		}
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   time.Second * time.Duration(conf.Timeout),
	}

	if !conf.Redirect {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client
}

// Get function
func Get(url string, headers []string, conf HTTPConf) ([]byte, error) {
	
	client := newClient(conf)

	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return nil, err
	}

	//if randomAgent {
		req.Header.Set("User-Agent", getUserAgent())
	//} else {
	//	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; linkz/1.0)")
	//}

	// add headers to the request
	if headers != nil{
		for _, h := range headers {
			parts := strings.SplitN(h, ":", 2)

			if len(parts) != 2 {
				continue
			}
			req.Header.Set(parts[0], parts[1])
		}
	}

	// send the request
	resp, err := client.Do(req)
	if err != nil {		
		return nil, err
	}
	defer resp.Body.Close()

	
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func getUserAgent() string {
	payload := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/74.0.3729.169 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/73.0.3683.103 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:66.0) Gecko/20100101 Firefox/66.0",
		"Mozilla/5.0 (Windows NT 6.2; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/68.0.3440.106 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_4) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/12.1 Safari/605.1.15",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/74.0.3729.131 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:67.0) Gecko/20100101 Firefox/67.0",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 8_4_1 like Mac OS X) AppleWebKit/600.1.4 (KHTML, like Gecko) Version/8.0 Mobile/12H321 Safari/600.1.4",
		"Mozilla/5.0 (Windows NT 10.0; WOW64; Trident/7.0; rv:11.0) like Gecko",
		"Mozilla/5.0 (iPad; CPU OS 7_1_2 like Mac OS X) AppleWebKit/537.51.2 (KHTML, like Gecko) Version/7.0 Mobile/11D257 Safari/9537.53",
		"Mozilla/5.0 (compatible; MSIE 10.0; Windows NT 6.1; Trident/6.0)",
	}

	rand.Seed(time.Now().UnixNano())
	randomIndex := rand.Intn(len(payload))

	pick := payload[randomIndex]

	return pick
}