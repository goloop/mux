package mux

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// registry remembers the application line each pattern was registered from.
//
// The standard mux reports a route conflict with the location of whoever
// called Handle, which for this package is always the single line inside
// register that every route passes through. Both halves of the conflict
// therefore point at the same, useless place. Recording the real caller here
// lets that location be put back before the panic reaches the application.
type registry struct {
	mu    sync.Mutex
	sites map[string]string
}

// remember records where pattern was registered from. A pattern registered
// twice keeps its first call site: that is the one the second registration
// conflicts with.
func (g *registry) remember(pattern, site string) {
	if g == nil || site == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sites == nil {
		g.sites = make(map[string]string)
	}
	if _, seen := g.sites[pattern]; !seen {
		g.sites[pattern] = site
	}
}

// lookup returns the call site recorded for pattern, or "" if there is none.
func (g *registry) lookup(pattern string) string {
	if g == nil {
		return ""
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.sites[pattern]
}

// retarget turns a panic raised by the standard mux during registration into
// one that points at the application.
//
// A conflict names both patterns and, for both, this package's own line; when
// both call sites are known, they are substituted in place and the rest of the
// standard explanation - which paths overlap, and why neither pattern is more
// specific - is left exactly as it was. Every other registration failure, such
// as a malformed pattern, is prefixed with the call site instead: there is no
// second route to point at. A panic value that is not an error, or a message
// this package no longer recognises, is passed through untouched rather than
// half-rewritten.
func (g *registry) retarget(v any, site string) any {
	err, ok := v.(error)
	if !ok {
		return v
	}
	if msg, ok := g.relocate(err.Error(), site); ok {
		return errors.New("mux: " + msg)
	}
	if site == "" {
		return v
	}
	return fmt.Errorf("mux: %s: %w", site, err)
}

// conflictSep separates the two patterns in the standard conflict message:
//
//	pattern "A" (registered at loc) conflicts with pattern "B" (registered at loc):
const conflictSep = " conflicts with pattern "

// locPrefix opens the parenthesis that holds a registration location.
const locPrefix = "(registered at "

// relocate rewrites both locations in a conflict message: the first belongs to
// the pattern being registered now, the second to the one already in the mux.
// It is all or nothing - a message that does not parse, or a pattern whose
// call site was never recorded, leaves the caller with the standard text
// rather than a half-corrected one.
func (g *registry) relocate(msg, site string) (string, bool) {
	if site == "" {
		return "", false
	}

	i := strings.Index(msg, conflictSep)
	if i < 0 {
		return "", false
	}
	quoted, err := strconv.QuotedPrefix(msg[i+len(conflictSep):])
	if err != nil {
		return "", false
	}
	other, err := strconv.Unquote(quoted)
	if err != nil {
		return "", false
	}
	otherSite := g.lookup(other)
	if otherSite == "" {
		return "", false
	}

	out, next, ok := replaceLoc(msg, 0, site)
	if !ok {
		return "", false
	}
	out, _, ok = replaceLoc(out, next, otherSite)
	if !ok {
		return "", false
	}
	return out, true
}

// replaceLoc swaps the first registration location at or after from for loc,
// and reports where to continue searching for the next one.
func replaceLoc(msg string, from int, loc string) (string, int, bool) {
	i := strings.Index(msg[from:], locPrefix)
	if i < 0 {
		return msg, 0, false
	}
	i += from + len(locPrefix)
	j := strings.IndexByte(msg[i:], ')')
	if j < 0 {
		return msg, 0, false
	}
	return msg[:i] + loc + msg[i+j:], i + len(loc), true
}

// callSite reports the file and line that registered a route: the first frame
// outside this package. The depth varies - Get goes through Method, Mount
// through mount, the *E helpers through HandleFunc - so the stack is walked
// rather than counted.
func callSite() string {
	var pcs [32]uintptr
	n := runtime.Callers(2, pcs[:]) // skip runtime.Callers and callSite
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if f.Function != "" && !strings.HasPrefix(f.Function, selfPackage) {
			return fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		if !more {
			return ""
		}
	}
}

// selfPackage is this package's symbol prefix, for example
// "github.com/goloop/mux.". It is read from the running binary rather than
// written down, so a vendored or forked copy is recognised as this package
// too. An empty value only costs precision: callSite then reports its own
// caller, which is where it would have stopped anyway for a direct Handle.
var selfPackage = func() string {
	pc, _, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	name := runtime.FuncForPC(pc).Name()
	slash := strings.LastIndexByte(name, '/')
	dot := strings.IndexByte(name[slash+1:], '.')
	if dot < 0 {
		return ""
	}
	return name[:slash+1+dot+1]
}()
