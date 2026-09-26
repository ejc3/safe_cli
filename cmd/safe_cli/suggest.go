package main

import "sort"

// closest returns the candidates within Levenshtein distance maxDist of target, nearest
// first, so an unknown name (e.g. a mistyped entity) can offer a "did you mean" hint. Ties
// keep the candidate order stable via a secondary sort by name.
func closest(target string, candidates []string, maxDist int) []string {
	type scored struct {
		name string
		d    int
	}
	var hits []scored
	for _, c := range candidates {
		if c == target {
			continue
		}
		if d := levenshtein(target, c); d <= maxDist {
			hits = append(hits, scored{c, d})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].d != hits[j].d {
			return hits[i].d < hits[j].d
		}
		return hits[i].name < hits[j].name
	})
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.name
	}
	return out
}

// levenshtein is the standard edit distance (insert/delete/substitute), used only for
// short command/entity names, so the O(n*m) row-buffer implementation is plenty.
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	prev := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	cur := make([]int, len(br)+1)
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
