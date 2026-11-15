package edit

import "testing"

func TestApply(t *testing.T) {
	doc := "<style>\n  .a { color: red; }\n  .b { color: blue; }\n</style>\n<p>x</p><p>x</p>\n"
	out, res := Apply(doc, []Edit{
		{Find: ".a { color: red; }", Replace: ".a { color: green; }"},        // exact
		{Find: "<p>x</p>", Replace: "<p>y</p>"},                              // ambiguous
		{Find: ".b { color: blue; }\n</style>", Replace: ".b{}\n</style>"},   // exact across lines
		{Find: "missing", Replace: "nothing"},                                // not found
		{Find: "    .a { color: green; }", Replace: "  .a { color: teal; }"}, // loose indentation
	})
	if res.Applied != 3 {
		t.Fatalf("applied %d, want 3; failures %+v", res.Applied, res.Failed)
	}
	if len(res.Failed) != 2 {
		t.Fatalf("failed %d, want 2", len(res.Failed))
	}
	want := "<style>\n  .a { color: teal; }\n  .b{}\n</style>\n<p>x</p><p>x</p>\n"
	if out != want {
		t.Fatalf("got\n%q\nwant\n%q", out, want)
	}
}
