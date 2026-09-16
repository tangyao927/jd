package nav

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type MatchKind string

const (
	MatchSubsequence MatchKind = "subsequence"
	MatchPrefix      MatchKind = "segment_prefix"
	MatchExactName   MatchKind = "exact_name"
)

type Candidate struct {
	Path        string
	Pin         string
	Sources     []string
	VisitCount  int
	LastVisited time.Time
}

type Match struct {
	Candidate
	Kind     MatchKind
	Frecency float64
}

func ResolveLiteral(args []string, cwd string) (string, bool, error) {
	if len(args) != 1 || args[0] == "" {
		return "", false, nil
	}
	input := args[0]
	if input == "~" || strings.HasPrefix(input, "~/") || strings.HasPrefix(input, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false, err
		}
		input = filepath.Join(home, strings.TrimLeft(input[1:], `/\`))
	}
	if !filepath.IsAbs(input) {
		input = filepath.Join(cwd, input)
	}
	path, err := filepath.Abs(filepath.Clean(input))
	if err != nil {
		return "", false, err
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.IsDir() {
		return "", false, nil
	}
	return path, true, nil
}

func ExactPin(query string, candidates []Candidate) (Candidate, bool) {
	for _, candidate := range candidates {
		if candidate.Pin != "" && strings.EqualFold(candidate.Pin, query) {
			return candidate, true
		}
	}
	return Candidate{}, false
}

func Rank(query []string, cwd string, candidates []Candidate, now time.Time) []Match {
	terms := normalizeTerms(query)
	matches := make([]Match, 0, len(candidates))
	for _, candidate := range candidates {
		kind, ok := classify(candidate.Path, terms)
		if !ok {
			continue
		}
		matches = append(matches, Match{
			Candidate: candidate,
			Kind:      kind,
			Frecency:  frecency(candidate.VisitCount, candidate.LastVisited, now),
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if matchWeight(a.Kind) != matchWeight(b.Kind) {
			return matchWeight(a.Kind) > matchWeight(b.Kind)
		}
		if (a.Pin != "") != (b.Pin != "") {
			return a.Pin != ""
		}
		if a.Frecency != b.Frecency {
			return a.Frecency > b.Frecency
		}
		ac, bc := commonAncestorDepth(cwd, a.Path), commonAncestorDepth(cwd, b.Path)
		if ac != bc {
			return ac > bc
		}
		as, bs := segmentCount(a.Path), segmentCount(b.Path)
		if as != bs {
			return as < bs
		}
		return strings.ToLower(a.Path) < strings.ToLower(b.Path)
	})
	return matches
}

func normalizeTerms(query []string) []string {
	terms := make([]string, 0, len(query))
	for _, term := range query {
		term = strings.ToLower(strings.TrimSpace(term))
		if term != "" {
			terms = append(terms, term)
		}
	}
	return terms
}

func classify(path string, terms []string) (MatchKind, bool) {
	if len(terms) == 0 {
		return MatchSubsequence, true
	}
	lower := strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
	base := strings.ToLower(filepath.Base(path))
	if len(terms) == 1 && base == terms[0] {
		return MatchExactName, true
	}
	pathRunes := []rune(lower)
	cursor := 0
	prefix := true
	segments := strings.Split(lower, "/")
	segmentCursor := 0
	for _, term := range terms {
		end, ok := subsequenceEnd(pathRunes, cursor, []rune(term))
		if !ok {
			return "", false
		}
		cursor = end
		foundPrefix := false
		for segmentCursor < len(segments) {
			segment := segments[segmentCursor]
			segmentCursor++
			if strings.HasPrefix(segment, term) {
				foundPrefix = true
				break
			}
		}
		prefix = prefix && foundPrefix
	}
	if prefix {
		return MatchPrefix, true
	}
	return MatchSubsequence, true
}

func subsequenceEnd(haystack []rune, start int, needle []rune) (int, bool) {
	if len(needle) == 0 {
		return start, true
	}
	matched := 0
	for index := start; index < len(haystack); index++ {
		if haystack[index] != needle[matched] {
			continue
		}
		matched++
		if matched == len(needle) {
			return index + 1, true
		}
	}
	return 0, false
}

func matchWeight(kind MatchKind) int {
	switch kind {
	case MatchExactName:
		return 3
	case MatchPrefix:
		return 2
	default:
		return 1
	}
}

func frecency(visits int, lastVisited, now time.Time) float64 {
	if visits <= 0 || lastVisited.IsZero() {
		return 0
	}
	age := now.Sub(lastVisited)
	multiplier := 0.5
	switch {
	case age < time.Hour:
		multiplier = 4
	case age < 24*time.Hour:
		multiplier = 2
	case age < 7*24*time.Hour:
		multiplier = 1
	}
	return math.Log2(float64(visits)+1) * multiplier
}

func commonAncestorDepth(a, b string) int {
	aParts := splitPath(a)
	bParts := splitPath(b)
	depth := 0
	for depth < len(aParts) && depth < len(bParts) && strings.EqualFold(aParts[depth], bParts[depth]) {
		depth++
	}
	return depth
}

func segmentCount(path string) int {
	return len(splitPath(path))
}

func splitPath(path string) []string {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	return strings.FieldsFunc(cleaned, func(r rune) bool { return r == '/' })
}
