package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// confirm writes a [y/N] prompt and returns true only if the user types "y" or "Y".
func confirm(r *bufio.Reader, out io.Writer, message string) (bool, error) {
	fmt.Fprintf(out, "%s [y/N]: ", message)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(line), "y"), nil
}
