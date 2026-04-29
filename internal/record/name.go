package record

import "fmt"

const MaxNodeNameLength = 15

func ValidateNodeName(name string) error {
	if name == "" {
		return fmt.Errorf("node name cannot be empty")
	}
	if len(name) > MaxNodeNameLength {
		return fmt.Errorf("node name %q exceeds %d characters", name, MaxNodeNameLength)
	}
	if name[0] == '-' || name[len(name)-1] == '-' {
		return fmt.Errorf("node name %q cannot start or end with a hyphen", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return fmt.Errorf("node name %q contains unsupported characters; use lowercase letters, digits, or hyphens", name)
		}
	}
	return nil
}
