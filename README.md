<h1 align="center">Linkz</h1>

> A golang utility to spider through a website searching for javascript files and inline code.

### Install
`go get -u github.com/bp0lr/linkz`


### why
Because I needed something fast, well programmed and with truthful results.

### top Features

* Multithread
* Build-in filter for common frameworks (jquery, angular, etc)
* save inline code
* filter links outside the main domain
* ability to crawl other file types

### basic usage

running gf agains what linkz found and dowload.

`cat subdomains.txt | linkz -f output --follow-redirect --timeout 10 -s`

```
for D in `find "output/" -type d`
do   
    for file in `find ${D} -type f`
    do
        for pattern in $(gf -list);
        do
            gf $pattern "${file}" | sort -u > "found_${pattern}.txt";
        done
    done             
done
```

## Contributing

Contributions, issues and feature requests are welcome!<br />Feel free to check [issues page](https://github.com/bp0lr/linkz/issue). 
