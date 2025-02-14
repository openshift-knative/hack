package util

import (
	"fmt"
	"regexp"
)

func ToRegexp(rawRegexps []string) ([]*regexp.Regexp, error) {
	regexps := make([]*regexp.Regexp, 0, len(rawRegexps))
	for _, i := range rawRegexps {
		r, err := regexp.Compile(i)
		if err != nil {
			return regexps, fmt.Errorf("regex %q doesn't compile: %w", i, err)
		}
		regexps = append(regexps, r)
	}
	return regexps, nil
}

func MustToRegexp(rawRegexps []string) []*regexp.Regexp {
	regex, err := ToRegexp(rawRegexps)
	if err != nil {
		panic(err)
	}
	return regex
}
