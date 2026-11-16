package contract

import "sort"

// Deviation is one way a file differs from the contract. Messages never
// include a config value: values may be secrets, and deviations end up in
// CI logs.
type Deviation struct {
	File string `json:"file"`
	Key  string `json:"key"`
	Kind string `json:"kind"` // stable, for tools; Msg is for people
	Msg  string `json:"message"`
}

func fileMissing(file string) Deviation {
	return Deviation{file, "", "file-missing", "file missing"}
}

func unexpectedFile(file string) Deviation {
	return Deviation{file, "", "unexpected-file", "unexpected file (not in contract)"}
}

func unreadable(file string, err error) Deviation {
	return Deviation{file, "", "unreadable", err.Error()}
}

func missingKey(file, key, typ string) Deviation {
	return Deviation{file, key, "missing-key", "missing key (" + typ + ")"}
}

func unknownKey(file, key string) Deviation {
	return Deviation{file, key, "unknown-key", "unknown key (not in contract)"}
}

func typeChanged(file, key, recorded, found string) Deviation {
	return Deviation{file, key, "type-changed", "type changed: " + recorded + " -> " + found}
}

func valueUnchanged(file, key string) Deviation {
	return Deviation{file, key, "value-unchanged", "value unchanged from snapshot (env-specific)"}
}

func valueChanged(file, key string) Deviation {
	return Deviation{file, key, "value-changed", "value differs from snapshot (locked)"}
}

func ruleMismatch(file, key, expr string) Deviation {
	return Deviation{file, key, "rule", "value does not match " + expr}
}

func ruleForbidden(file, key, expr string) Deviation {
	return Deviation{file, key, "rule", "value matches forbidden " + expr}
}

// Stale says that a contract file no longer matches what a snapshot would write.
func Stale(contractFile string) Deviation {
	return Deviation{contractFile, "", "stale-contract", "contract is stale: run contracter snapshot"}
}

// NotWritten says that a contract file a snapshot would write does not exist.
func NotWritten(contractFile string) Deviation {
	return Deviation{contractFile, "", "stale-contract", "contract does not exist: run contracter snapshot"}
}

func sorted(devs []Deviation) []Deviation {
	sort.Slice(devs, func(i, j int) bool {
		a, b := devs[i], devs[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		return a.Msg < b.Msg
	})
	return devs
}
