// only pure code in this file (no side effects)
package typer

import (
	"bufio"
	"bytes"
	"io"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
)

// PhraseFunc generates a phrase from a seed and returns the next seed.
type PhraseFunc func(seed int64) (int64, string)

// DefaultPhrase generates a built-in fallback phrase.
func DefaultPhrase(seed int64) (int64, string) {
	return seed, "the quick brown fox jumps over the lazy dog"
}

// StaticPhrase returns a static phrase generator function with the given phrase.
func StaticPhrase(phrase string) PhraseFunc {
	return func(seed int64) (int64, string) {
		return seed, phrase
	}
}

// RandomPhrase composes a random phrase with given length from given words.
func RandomPhrase(words []string, minLength int, numProb float64) PhraseFunc {
	return func(seed int64) (int64, string) {
		r := rand.New(rand.NewSource(seed))
		var phrase []string
		l := -1
		for l < minLength {
			var w string
			if r.Float64() < numProb {
				w = strconv.FormatInt(r.Int63n(10000), 10)
			} else {
				w = words[r.Int31n(int32(len(words)))]
			}
			phrase = append(phrase, w)
			l += 1 + len(w)
		}
		return r.Int63(), strings.Join(phrase, " ")
	}
}

// SequentialLine goes through a sequence of lines sequentially.
func SequentialLine(lines []string) PhraseFunc {
	return func(seed int64) (int64, string) {
		line := lines[seed%int64(len(lines))]
		return (seed + 1) % int64(len(lines)), line
	}
}

// RandomLine picks a random line from lines on each call.
func RandomLine(lines []string) PhraseFunc {
	return func(seed int64) (int64, string) {
		r := rand.New(rand.NewSource(seed))
		line := lines[r.Int63n(int64(len(lines)))]
		return r.Int63(), line
	}
}

func filterWords(words []string, pattern string, maxLength int) []string {
	filtered := make([]string, 0)
	compiled := regexp.MustCompile(pattern)

	for _, word := range words {
		trimmed := strings.TrimSpace(word)
		if compiled.MatchString(trimmed) && len(trimmed) <= maxLength {
			filtered = append(filtered, trimmed)
		}
	}

	return filtered
}

func readLines(data []byte) []string {
	var lines []string

	reader := bufio.NewReader(bytes.NewBuffer(data))
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return lines
			}
			panic(err)
		}
		lines = append(lines, line[:len(line)-1])
	}
}
