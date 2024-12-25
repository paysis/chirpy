package main

import (
	"fmt"
	"testing"
)

func TestFilterProfaneWords(t *testing.T) {
	profaneList := []string{
		"kerfuffle",
		"sharbert",
		"fornax",
	}

	cases := []struct {
		input          string
		profanes       []string
		expectedOutput string
	}{
		{
			input:          "I had something interesting for breakfast",
			profanes:       profaneList,
			expectedOutput: "I had something interesting for breakfast",
		},
		{
			input:          "I hear Mastodon is better than Chirpy. sharbert I need to migrate",
			profanes:       profaneList,
			expectedOutput: "I hear Mastodon is better than Chirpy. **** I need to migrate",
		},
		{
			input:          "I really need a kerfuffle to go to bed sooner, Fornax !",
			profanes:       profaneList,
			expectedOutput: "I really need a **** to go to bed sooner, **** !",
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Case %d", i+1), func(t *testing.T) {
			output := filterProfaneWords(c.input, c.profanes)

			if output != c.expectedOutput {
				t.Errorf("expected \"%v\", have got: \"%v\"\n", c.expectedOutput, output)
			}
		})
	}

}
