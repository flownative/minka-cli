package cli

import (
	"maps"
	"os"
	"regexp"
	"strconv"
	"testing"
)

// exitCodeContract is the table of exit codes as the documentation promises
// it. Scripts key on these numbers, so a change here is a change of contract.
var exitCodeContract = map[string]int{
	CodeUsage:      1,
	CodeConfig:     1,
	CodeNotFound:   2,
	CodeAuth:       3,
	CodeForbidden:  4,
	CodeNetwork:    6,
	CodeAPI:        7,
	CodeAmbiguous:  8,
	CodeValidation: 9,
	CodeUnread:     11,
	CodeTimeout:    12,
}

// The safeguard: every code leaves with the exit code the contract names, and
// no code leaves with one the contract does not know.
func TestEveryCodeExitsAsTheContractSays(t *testing.T) {
	for code, want := range exitCodeContract {
		if got := (&Error{Code: code}).ExitCode(); got != want {
			t.Errorf("%s exits with %d, the contract says %d", code, got, want)
		}
	}
	if !maps.Equal(exitCodes, exitCodeContract) {
		t.Errorf("the exit codes are %v, the contract %v", exitCodes, exitCodeContract)
	}
}

// docRow is a row of a table of exit codes: the number, then the codes.
var docRow = regexp.MustCompile("(?m)^\\| (\\d+) \\| ([^|]*)\\|")
var docCode = regexp.MustCompile("`([a-z_]+)`")

// Every document that lists the exit codes lists the contract, row by row.
func TestTheDocumentedExitCodesAreTheContract(t *testing.T) {
	for _, path := range []string{"../../docs/stable/output-contract.md", "../../README.md", "../../skills/minka/SKILL.md"} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		documented := map[string]int{}
		for _, row := range docRow.FindAllStringSubmatch(string(content), -1) {
			exit, _ := strconv.Atoi(row[1])
			for _, code := range docCode.FindAllStringSubmatch(row[2], -1) {
				documented[code[1]] = exit
			}
		}
		if !maps.Equal(documented, exitCodeContract) {
			t.Errorf("%s documents %v, the contract is %v", path, documented, exitCodeContract)
		}
	}
}
