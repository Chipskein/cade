package devtasks

import "fmt"

// fuzzTarget is a fuzz function in a package under internal/.
type fuzzTarget struct {
	pkg, name string
}

// fuzzTargets cover the Teams cache parsers; what a run finds lands in
// testdata/fuzz/ and is replayed by every `go tool mage test`.
var fuzzTargets = []fuzzTarget{
	{"leveldbraw", "FuzzJournalBatches"}, {"leveldbraw", "FuzzDecodeBatch"}, {"leveldbraw", "FuzzTableEntries"},
	{"leveldbraw", "FuzzBlockEntries"}, {"v8value", "FuzzDecode"}, {"indexeddb", "FuzzDecodeKeyPrefix"},
	{"indexeddb", "FuzzDecodeRecords"}, {"ingest/teamssource", "FuzzCollectReplyChain"},
}

// Fuzz runs each target for FUZZTIME, stopping at the first failure.
func (t *Tasks) Fuzz() error {
	for _, target := range fuzzTargets {
		fmt.Fprintf(t.progress, "== %s %s\n", target.pkg, target.name)
		if err := t.runner.Run(t.fuzzCommand(target)); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tasks) fuzzCommand(target fuzzTarget) Command {
	return Command{Name: "go", Args: []string{
		"test", "-tags", fts5Tag, "-run", "^$", "-fuzz", "^" + target.name + "$",
		"-fuzztime", t.settings.FuzzTime, "./internal/" + target.pkg,
	}}
}
