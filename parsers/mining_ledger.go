package parsers

import (
	"fmt"
	"sort"
	"strings"
)

var miningLedgerHeader = "Timestamp	Ore Type	Quantity	Volume	Est. Price	Solar System	Ore TypeID	SolarSystemID"

type MiningLedger struct {
	Items []MiningLedgerItem
	lines []int
}

func (r *MiningLedger) Name() string {
	return "mining_ledger"
}

func (r *MiningLedger) Lines() []int {
	return r.lines
}

type MiningLedgerItem struct {
	Name     string
	Quantity int64
}

func ParseMiningLedger(input Input) (ParserResult, Input) {
	ledger := &MiningLedger{}

	if len(input) == 0 {
		return nil, input
	}

	if input[0] != miningLedgerHeader {
		return nil, input
	}
	ledger.lines = []int{0}
	rest := make(Input)
	inputLines := input.Strings()
	matchgroup := make(map[MiningLedgerItem]int64)
	for i, line := range inputLines[1:] {
		parts := strings.Split(line, "\t")
		if len(parts) != 8 {
			rest[i+1] = line
			continue
		}
		matchgroup[MiningLedgerItem{Name: CleanTypeName(parts[1])}] += ToInt(parts[2])
		ledger.lines = append(ledger.lines, i+1)
	}

	for item, quantity := range matchgroup {
		item.Quantity = quantity
		ledger.Items = append(ledger.Items, item)
	}

	sort.Slice(ledger.Items, func(i, j int) bool {
		return fmt.Sprintf("%v", ledger.Items[i]) < fmt.Sprintf("%v", ledger.Items[j])
	})
	sort.Ints(ledger.lines)
	return ledger, rest
}
