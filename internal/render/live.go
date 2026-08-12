package render

import (
	"fmt"
	"strings"
)

// Live redraws a block of output in place, for a stream that updates rather than
// scrolls. On a terminal it rewinds over the previous block and overwrites it; on
// a pipe there is no cursor to move, so each frame is simply appended, which
// keeps a redirected stream readable as a log.
type Live struct {
	p     *Printer
	lines int
}

// Live starts an in-place renderer on this stream.
func (p *Printer) Live() *Live { return &Live{p: p} }

// Render replaces the previous frame with content.
func (l *Live) Render(content string) {
	content = strings.TrimRight(content, "\n")

	if !l.p.TTY {
		fmt.Fprintln(l.p.w, content)
		return
	}

	if l.lines > 0 {
		// Move the cursor to the start of the previous frame and clear from
		// there to the end of the screen.
		fmt.Fprintf(l.p.w, "\x1b[%dA\x1b[0J", l.lines)
	}

	fmt.Fprintln(l.p.w, content)
	l.lines = strings.Count(content, "\n") + 1
}

// Finish leaves the last frame on screen and stops tracking it, so anything
// printed afterwards appends rather than overwriting.
func (l *Live) Finish() { l.lines = 0 }
