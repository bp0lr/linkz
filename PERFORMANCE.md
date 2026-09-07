# Local performance checks

Measured on Windows amd64, Go 1.26.8, AMD Ryzen 9 3900X. These are microbenchmarks of the library filter, not end-to-end network speed claims.

```sh
go test ./static -run '^$' -bench BenchmarkExist -benchmem -benchtime=200ms -count=3
go test . -run '^$' -bench BenchmarkExtract -benchmem -benchtime=200ms
```

Median of three runs for library lookup, before and after replacing repeated variant generation with a precomputed map:

| Filename | Before ns/op | After ns/op | Before allocations/op | After allocations/op |
| --- | ---: | ---: | ---: | ---: |
| `app.js` | 241223 | 15.34 | 5000 | 0 |
| `jquery.min.js` | 182629 | 28.19 | 3680 | 0 |
| `react.js` | 28082 | 20.25 | 576 | 0 |

Map construction happens once at startup and is outside the lookup benchmark. Lowercase lookups allocate no memory in these measurements; converting uppercase input may allocate.

The HTML extraction baseline for a page with 64 script tags was 267898 ns/op, 123028 B/op, and 1825 allocations/op. It is a baseline for future work, not a before/after comparison.

Regression tests also verify that:

- Two distinct pages referencing one script make only one script request.
- Duplicate input page URLs, including fragment-only differences, are fetched once.
- Four sequential requests reuse one local HTTP connection.
- A failed streamed write preserves the previous complete file and removes its temporary file.

Network latency, response sizes, unique URL count, and filesystem performance determine real collection time and memory usage. HTML bodies remain buffered up to the configured response limit per active worker; scripts stream directly into temporary files.
