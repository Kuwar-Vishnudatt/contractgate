// Command contractgate is the CLI: `check` is the CI gate (compares two
// contract versions and fails if a compatibility violation isn't paired
// with a major version bump), `validate` checks a JSON data file against a
// contract, and `demo` runs a multi-producer ingestion scenario end to end.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/Kuwar-Vishnudatt/contractgate/internal/compat"
	"github.com/Kuwar-Vishnudatt/contractgate/internal/contract"
	"github.com/Kuwar-Vishnudatt/contractgate/internal/validate"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "check":
		runCheck(os.Args[2:])
	case "validate":
		runValidate(os.Args[2:])
	case "demo":
		runDemo(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: contractgate <check|validate|demo> [flags]")
}

func runCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	oldPath := fs.String("old", "", "path to the previous contract version (required)")
	newPath := fs.String("new", "", "path to the proposed new contract version (required)")
	fs.Parse(args)

	if *oldPath == "" || *newPath == "" {
		fmt.Fprintln(os.Stderr, "check: -old and -new are required")
		os.Exit(2)
	}

	oldC, err := contract.Load(*oldPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	newC, err := contract.Load(*newPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	decision, err := compat.Evaluate(oldC, newC)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	fmt.Printf("contract %q: %s -> %s (mode: %s)\n", newC.Name, oldC.Version, newC.Version, decision.EffectiveMode)
	if len(decision.Report.BackwardIssues) > 0 {
		fmt.Println("  backward-compatibility issues:")
		for _, i := range decision.Report.BackwardIssues {
			fmt.Printf("    - [%s] %s\n", i.Kind, i.Message)
		}
	}
	if len(decision.Report.ForwardIssues) > 0 {
		fmt.Println("  forward-compatibility issues:")
		for _, i := range decision.Report.ForwardIssues {
			fmt.Printf("    - [%s] %s\n", i.Kind, i.Message)
		}
	}

	if decision.Blocked {
		fmt.Println("\nBLOCKED:")
		for _, r := range decision.Reasons {
			fmt.Println("  -", r)
		}
		fmt.Println("\nFix: bump the major version component if this change is intentional, or revise the contract to avoid the violation.")
		os.Exit(1)
	}

	fmt.Println("\nOK: no blocking compatibility violations under the enforced mode.")
}

func runValidate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	contractPath := fs.String("contract", "", "path to the contract (required)")
	dataPath := fs.String("data", "", "path to a JSON file: one object, or an array of objects (required)")
	fs.Parse(args)

	if *contractPath == "" || *dataPath == "" {
		fmt.Fprintln(os.Stderr, "validate: -contract and -data are required")
		os.Exit(2)
	}

	c, err := contract.Load(*contractPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	raw, err := os.ReadFile(*dataPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	events, err := decodeEvents(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	anyFailed := false
	for i, e := range events {
		violations := validate.Event(c, e)
		if len(violations) == 0 {
			fmt.Printf("event %d: OK\n", i)
			continue
		}
		anyFailed = true
		fmt.Printf("event %d: %d violation(s)\n", i, len(violations))
		for _, v := range violations {
			fmt.Printf("  - %s\n", v)
		}
	}

	if anyFailed {
		os.Exit(1)
	}
}

func decodeEvents(raw []byte) ([]map[string]any, error) {
	var asArray []map[string]any
	if err := json.Unmarshal(raw, &asArray); err == nil {
		return asArray, nil
	}
	var asOne map[string]any
	if err := json.Unmarshal(raw, &asOne); err != nil {
		return nil, fmt.Errorf("data must be a JSON object or array of objects: %w", err)
	}
	return []map[string]any{asOne}, nil
}
