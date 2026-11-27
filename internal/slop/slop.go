// Package slop finds patterns that are almost always generic AI output (aitk standards anti-slop).
package slop

// aitk-slop: ignore-file (this file lists the patterns it looks for)

import (
	"bufio"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
)

// Hit is one line that looks like generic AI output.
type Hit struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Rule    string `json:"rule"`
	Why     string `json:"why"`
	Excerpt string `json:"excerpt"`
}

type slopRule struct {
	id, why string
	re      *regexp.Regexp
	scope   string // "text": markdown and comments; "comment": code comments; "code": code lines; "md": markdown only
}

// The rules are deliberately narrow: each one names a pattern that is almost always filler,
// a swallowed error, or a leftover placeholder, so a hit is worth a look. They advise; they never block.
var slopRules = []slopRule{
	{"filler-phrase", "marketing filler that says nothing; state the concrete behaviour instead", regexp.MustCompile(`(?i)\b(delve|delving|seamless(ly)?|cutting-edge|game[- ]changer|best-in-class|unlock the (full )?power|elevate your|in today's fast-paced|it'?s worth noting that|a testament to|rich tapestry|embark on (a|this|the) journey|robust and scalable|supercharge|revolutioni[sz]e)\b`), "text"},
	{"assistant-voice", "chat-assistant phrasing left in a file", regexp.MustCompile(`(?i)(\bas an ai\b|\bi hope this helps\b|^\s*(certainly|absolutely)!|\bgreat question\b|\bhappy coding\b)`), "text"},
	{"emoji-heading", "decorative emoji in headings or bullets; plain words read better and diff cleanly", regexp.MustCompile(`^\s*(#{1,6}|[-*])\s.*[🚀✨🔥💯🎉🌟⚡️💥🙌]`), "md"},
	{"restating-comment", "the comment repeats what the next line already says; explain why, or delete it", regexp.MustCompile(`(?i)^\s*(//|#|--|\*)\s*(increment(s)? |decrement(s)? |return(s)? the (result|value)|loop (through|over) |iterate (through|over) |import (the )?(necessary|required) |define (a|the) (function|variable|constant)|initiali[sz]e (a|the) (variable|counter)|call the function|create a new instance)`), "comment"},
	{"swallowed-error", "the error is caught and dropped; handle it, return it, or log it with context", regexp.MustCompile(`(catch\s*(\([^)]*\))?\s*\{\s*\}|except(\s+\w+)?\s*:\s*pass\b|_\s*=\s*err\b\s*$|rescue\s*(=>\s*\w+)?\s*;?\s*end\b)`), "code"},
	{"placeholder", "placeholder text left in the change", regexp.MustCompile(`(?i)(lorem ipsum|your[-_ ]api[-_ ]key[-_ ]here|todo:? implement( this| me)?\b|replace with (your|actual) )`), "text"},
}

var slopCodeExt = map[string]bool{".go": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".py": true, ".rb": true, ".php": true,
	".java": true, ".kt": true, ".cs": true, ".rs": true, ".swift": true, ".c": true, ".h": true, ".cpp": true, ".sql": true, ".sh": true,
	".vue": true, ".svelte": true, ".dart": true, ".scala": true}

const maxSlopFileBytes = 1 << 20

// Scan checks the given repository-relative paths. A file containing "aitk-slop: ignore-file" is
// skipped, and a line containing "aitk-slop: ignore" is not reported.
func Scan(p *project.Project, paths []string) []Hit {
	var hits []Hit
	for _, rel := range paths {
		hits = append(hits, scanFile(p, rel)...)
	}
	return hits
}

func scanFile(p *project.Project, rel string) []Hit {
	ext := strings.ToLower(path.Ext(rel))
	isMD, isCode := ext == ".md" || ext == ".mdx", slopCodeExt[ext]
	if !isMD && !isCode {
		return nil
	}
	f, err := os.Open(p.Path(rel))
	if err != nil {
		return nil
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || st.Size() > maxSlopFileBytes {
		return nil
	}
	var hits []Hit
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), maxSlopFileBytes)
	inFence := false
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if strings.Contains(line, "aitk-slop: ignore-file") {
			return nil
		}
		if strings.Contains(line, "aitk-slop: ignore") {
			continue
		}
		if isMD && strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		comment := isCode && isCommentLine(line)
		for _, r := range slopRules {
			applies := false
			switch r.scope {
			case "md":
				applies = isMD && !inFence
			case "text":
				applies = (isMD && !inFence) || comment
			case "comment":
				applies = comment
			case "code":
				applies = (isCode && !comment) || (isMD && inFence)
			}
			if applies && r.re.MatchString(line) {
				hits = append(hits, Hit{File: rel, Line: n, Rule: r.id, Why: r.why, Excerpt: excerpt(line)})
			}
		}
	}
	return hits
}

func isCommentLine(l string) bool {
	t := strings.TrimSpace(l)
	for _, pre := range []string{"//", "#", "--", "/*", "*", "<!--"} {
		if strings.HasPrefix(t, pre) {
			return true
		}
	}
	return false
}

func excerpt(l string) string {
	t := strings.TrimSpace(l)
	if r := []rune(t); len(r) > 100 {
		return string(r[:100]) + "…"
	}
	return t
}
