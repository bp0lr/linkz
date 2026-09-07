package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func BenchmarkExtract(b *testing.B) {
	var html strings.Builder
	for i := 0; i < 64; i++ {
		fmt.Fprintf(&html, `<script src="/app%d.js"></script>`, i)
	}
	source := []byte(html.String())
	b.ReportAllocs()
	for b.Loop() {
		if _, err := extract("https://example.test/", source, false, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func TestExtractReferences(t *testing.T) {
	source := []byte(`<base href="/assets/"><script src="app.js?v=1&amp;x=2"></script><script type="module" src="./module.mjs"></script><script src="/serve?id=1"></script><script src="jquery.min.js?v=2"></script><script>const other = "../extra.js";</script>`)
	got, err := extract("https://example.test/pages/index.html", source, false, nil)
	want := []string{"https://example.test/assets/app.js?v=1&x=2", "https://example.test/assets/module.mjs", "https://example.test/serve?id=1", "https://example.test/extra.js"}
	if err != nil || !reflect.DeepEqual(got.links, want) {
		t.Fatalf("links=%v err=%v", got.links, err)
	}
	if len(got.inline) != 1 || got.inline[0].index != 5 {
		t.Fatalf("inline=%v", got.inline)
	}
}

func TestExtractOriginAndInlineTypes(t *testing.T) {
	source := []byte(`<script src="https://cdn.test/app.js"></script><script src="data:text/javascript,foo"></script><script type="application/json">{"x":1}</script><script type="module">export {};</script><script src="/external.js">fallback</script>`)
	got, err := extract("https://example.test/", source, false, nil)
	if err != nil || !reflect.DeepEqual(got.links, []string{"https://example.test/external.js"}) || len(got.inline) != 1 || string(got.inline[0].code) != "export {};" {
		t.Fatalf("%+v err=%v", got, err)
	}
}
