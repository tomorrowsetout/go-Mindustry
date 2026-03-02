package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type entry struct {
	File       string
	Annotation string
	Signature  string
}

func main() {
	root := filepath.Join("..", "..", "core", "src")
	outPath := filepath.Join("..", "..", "go-server", "docs", "remote-methods.csv")

	var entries []entry

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".java") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		var lastRemote string
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "@Remote") {
				lastRemote = line
				continue
			}
			if lastRemote != "" {
				if line == "" || strings.HasPrefix(line, "@") {
					continue
				}
				entries = append(entries, entry{
					File:       path,
					Annotation: lastRemote,
					Signature:  line,
				})
				lastRemote = ""
			}
		}
		return scanner.Err()
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan failed: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir failed: %v\n", err)
		os.Exit(1)
	}

	out, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create failed: %v\n", err)
		os.Exit(1)
	}
	defer out.Close()

	w := csv.NewWriter(out)
	_ = w.Write([]string{"file", "remote", "signature"})
	for _, e := range entries {
		_ = w.Write([]string{filepath.ToSlash(e.File), e.Annotation, e.Signature})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %d entries to %s\n", len(entries), outPath)
}
