// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package indexer

type SearchModeType string

const (
	SearchModeExact  SearchModeType = "exact"
	SearchModeWords  SearchModeType = "words"
	SearchModeFuzzy  SearchModeType = "fuzzy"
	SearchModeRegexp SearchModeType = "regexp"
)

type SearchMode struct {
	ModeValue    SearchModeType
	TooltipTrKey string
	TitleTrKey   string
}

func SearchModesExactWords() []SearchMode {
	return []SearchMode{
		{
			ModeValue:    SearchModeExact,
			TooltipTrKey: "Include only results that match the exact search term",
			TitleTrKey:   "Exact",
		},
		{
			ModeValue:    SearchModeWords,
			TooltipTrKey: "Include only results that match the search term words",
			TitleTrKey:   "Words",
		},
	}
}

func SearchModesExactWordsFuzzy() []SearchMode {
	return append(SearchModesExactWords(), []SearchMode{
		{
			ModeValue:    SearchModeFuzzy,
			TooltipTrKey: "Include results that closely match the search term",
			TitleTrKey:   "Fuzzy",
		},
	}...)
}

func GitGrepSupportedSearchModes() []SearchMode {
	return append(SearchModesExactWords(), []SearchMode{
		{
			ModeValue:    SearchModeRegexp,
			TooltipTrKey: "Include only results that match the regexp search term",
			TitleTrKey:   "Regexp",
		},
	}...)
}
