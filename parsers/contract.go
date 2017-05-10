package parsers

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Contract struct {
	Items []ContractItem
	lines []int
}

func (r *Contract) Name() string {
	return "contract"
}

func (r *Contract) Lines() []int {
	return r.lines
}

type ContractItem struct {
	Name     string
	Quantity int64
	Type     string
	Category string
	Details  string
	Fitted   bool
}

var reContract = regexp.MustCompile(strings.Join([]string{
	`^([\S ]*)\t`,   // Name
	`([\d,'\.]*)\t`, // Quantity
	`([\S ]*)\t`,    // type
	`([\S ]*)\t`,    // Category
	`([\S ]*)$`,     // Details
}, ""))

var reContractShort = regexp.MustCompile(strings.Join([]string{
	`^([\S ]*)\t`,   // Name
	`([\d,'\.]*)\t`, // Quantity
	`([\S ]*)$`,     // type
}, ""))

func ParseContract(input Input) (ParserResult, Input) {
	contract := &Contract{}
	matches, rest := regexParseLines(reContract, input)
	matches2, rest := regexParseLines(reContractShort, rest)
	contract.lines = append(regexMatchedLines(matches), regexMatchedLines(matches2)...)

	// collect items
	matchgroup := make(map[ContractItem]int64)
	for _, match := range matches {
		item := ContractItem{
			Name:     match[1],
			Type:     match[3],
			Category: match[4],
			Details:  match[5],
			Fitted:   strings.HasPrefix(match[5], "Fitted"),
		}

		matchgroup[item] += ToInt(match[2])
	}

	for _, match := range matches2 {
		item := ContractItem{
			Name: match[1],
			Type: match[3],
		}
		matchgroup[item] += ToInt(match[2])
	}

	// add items w/totals
	for item, Quantity := range matchgroup {
		item.Quantity = Quantity
		contract.Items = append(contract.Items, item)
	}

	sort.Slice(contract.Items, func(i, j int) bool {
		return fmt.Sprintf("%v", contract.Items[i]) < fmt.Sprintf("%v", contract.Items[j])
	})
	return contract, rest
}