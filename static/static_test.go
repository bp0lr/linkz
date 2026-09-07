package statics

import "testing"

func TestLibraryVariants(t *testing.T) {
	for _, name := range []string{"jquery.js", "jquery.min.js", "min.jquery.js", "JQUERY.MIN.JS", "react.js"} {
		if !Exist(name) {
			t.Errorf("missing %s", name)
		}
	}
	for _, name := range []string{"app.js", "myjquery.js", "jquery.min.js.backup"} {
		if Exist(name) {
			t.Errorf("excluded %s", name)
		}
	}
}

func BenchmarkExist(b *testing.B) {
	for _, name := range []string{"app.js", "jquery.min.js", "react.js"} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Exist(name)
			}
		})
	}
}
