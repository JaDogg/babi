// only pure code in this file (no side effects)
package typer

import (
	"math"
	"time"
	"unicode/utf8"
)

const (
	speedErrorRatio = 0.2
	scoreScalar     = 100.
	scoreExponent   = 2.3
	scoreCharFactor = 10.
)

// speedScore returns a score between 0 and 1, where 0.5 is at 2 CPS.
func speedScore(text string, d time.Duration) float64 {
	spc := d.Seconds() / float64(utf8.RuneCountInString(text))
	return 1. / (1. + 2*spc)
}

// errorScore returns a score between 0 and 1, where 0.5 is at 1 error.
func errorScore(errors int) float64 {
	return 1. / (1. + float64(errors))
}

func combinedScore(text string, d time.Duration, errors int) float64 {
	return speedErrorRatio*speedScore(text, d) +
		(1-speedErrorRatio)*errorScore(errors)
}

func finalScore(text string, fast, slow, normal float64) float64 {
	return maxScore(text) * math.Pow(0.15*fast+0.35*slow+0.5*normal, 2)
}

func maxScore(text string) float64 {
	return scoreCharFactor * float64(utf8.RuneCountInString(text))
}

func requiredScore(lvl int) float64 {
	return scoreScalar * math.Pow(float64(lvl), scoreExponent)
}

func scoreLevel(score float64) int {
	return int(math.Pow(score/scoreScalar, 1./scoreExponent))
}

func scoreProgress(score float64) float64 {
	current := scoreLevel(score)
	currentRequired := requiredScore(current)
	nextRequired := requiredScore(current + 1)
	return (score - currentRequired) / (nextRequired - currentRequired)
}
