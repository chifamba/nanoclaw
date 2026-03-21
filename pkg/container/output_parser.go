package container

import (
	"encoding/json"
	"strings"

	"github.com/nanoclaw/nanoclaw/pkg/logger"
)

// Sentinel markers for robust output parsing
const (
	OutputStartMarker = "---NANOCLAW_OUTPUT_START---"
	OutputEndMarker   = "---NANOCLAW_OUTPUT_END---"
)

// OutputParser handles streaming output parsing
type OutputParser struct {
	buffer             string
	OnOutput           func(ContainerOutput)
	OnActivityDetected func()
}

// ParseChunk processes a new chunk of data from stdout
func (p *OutputParser) ParseChunk(chunk string) {
	p.buffer += chunk

	for {
		startIdx := strings.Index(p.buffer, OutputStartMarker)
		if startIdx == -1 {
			break
		}

		endIdx := strings.Index(p.buffer[startIdx+len(OutputStartMarker):], OutputEndMarker)
		if endIdx == -1 {
			break
		}
		endIdx += startIdx + len(OutputStartMarker)

		jsonStr := strings.TrimSpace(p.buffer[startIdx+len(OutputStartMarker) : endIdx])
		p.buffer = p.buffer[endIdx+len(OutputEndMarker):]

		var output ContainerOutput
		if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
			logger.Warn("Failed to parse streamed output chunk", "error", err, "json", jsonStr)
			continue
		}

		if p.OnActivityDetected != nil {
			p.OnActivityDetected()
		}

		if p.OnOutput != nil {
			p.OnOutput(output)
		}
	}
}

// GetLastOutput extracts the last valid output from a complete buffer (for non-streaming fallback)
func GetLastOutput(stdout string) (ContainerOutput, error) {
	startIdx := strings.LastIndex(stdout, OutputStartMarker)
	endIdx := strings.LastIndex(stdout, OutputEndMarker)

	var jsonStr string
	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		jsonStr = strings.TrimSpace(stdout[startIdx+len(OutputStartMarker) : endIdx])
	} else {
		// Fallback: last non-empty line
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) > 0 {
			jsonStr = lines[len(lines)-1]
		}
	}

	var output ContainerOutput
	err := json.Unmarshal([]byte(jsonStr), &output)
	return output, err
}
