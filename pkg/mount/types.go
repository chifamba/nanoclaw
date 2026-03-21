package mount

// AdditionalMount represents a mount requested by a container
type AdditionalMount struct {
	HostPath      string `json:"hostPath"`                // Absolute path on host (supports ~ for home)
	ContainerPath string `json:"containerPath,omitempty"` // Optional — defaults to basename of hostPath
	Readonly      *bool  `json:"readonly,omitempty"`      // Default: true for safety
}

// AllowedRoot defines a directory that can be mounted into containers
type AllowedRoot struct {
	Path           string `json:"path"`
	AllowReadWrite bool   `json:"allowReadWrite"`
	Description    string `json:"description,omitempty"`
}

// MountAllowlist defines the security configuration for additional mounts
type MountAllowlist struct {
	AllowedRoots    []AllowedRoot `json:"allowedRoots"`
	BlockedPatterns []string      `json:"blockedPatterns"`
	NonMainReadOnly bool          `json:"nonMainReadOnly"`
}

// MountValidationResult contains the result of validating a single mount
type MountValidationResult struct {
	Allowed               bool   `json:"allowed"`
	Reason                string `json:"reason"`
	RealHostPath          string `json:"realHostPath,omitempty"`
	ResolvedContainerPath string `json:"resolvedContainerPath,omitempty"`
	EffectiveReadonly     bool   `json:"effectiveReadonly"`
}

// ValidatedMount represents a mount that has passed validation
type ValidatedMount struct {
	HostPath      string `json:"hostPath"`
	ContainerPath string `json:"containerPath"`
	Readonly      bool   `json:"readonly"`
}
