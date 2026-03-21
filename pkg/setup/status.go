package setup

import (
	"fmt"
	"sort"
)

// EmitStatus outputs a structured status block for setup steps.
// Each step emits a block that the SKILL.md LLM can parse.
func EmitStatus(step string, fields map[string]interface{}) {
	fmt.Printf("=== NANOCLAW SETUP: %s ===\n", step)

	// Sort keys for deterministic output
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Printf("%s: %v\n", k, fields[k])
	}
	fmt.Println("=== END ===")
}
