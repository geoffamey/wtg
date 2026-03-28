package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// prompt writes a prompt to out and reads one line from r.
// If the user enters nothing (or EOF is reached), defaultVal is returned.
func prompt(r *bufio.Reader, out io.Writer, message, defaultVal string) (string, error) {
	if defaultVal != "" {
		fmt.Fprintf(out, "%s [%s]: ", message, defaultVal) //nolint:errcheck
	} else {
		fmt.Fprintf(out, "%s: ", message) //nolint:errcheck
	}

	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal, nil
	}
	return line, nil
}

// confirm writes a [y/N] prompt and returns true only if the user types "y" or "Y".
func confirm(r *bufio.Reader, out io.Writer, message string) (bool, error) {
	fmt.Fprintf(out, "%s [y/N]: ", message) //nolint:errcheck
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(line), "y"), nil
}
