package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type methodEntry struct {
	Index       int
	File        string
	Remote      string
	Signature   string
	MethodName  string
	ParamTypes  []string
	CalledServer bool
}

type packetInfo struct {
	ClassName string
	Fields    []string
}

func main() {
	jar := filepath.Clean(`W:\steam\steam\steamapps\common\Mindustry\jre\desktop.jar154`)
	javap := filepath.Clean(`C:\Program Files\Java\jdk-12\bin\javap.exe`)
	csvIn := filepath.Clean(`..\..\go-server\docs\remote-methods.csv`)
	csvOut := filepath.Clean(`..\..\go-server\docs\remote-methods-ordered.csv`)

	methods, err := loadMethods(csvIn)
	if err != nil {
		fail(err)
	}

	order, err := callPacketOrder(javap, jar)
	if err != nil {
		fail(err)
	}

	packetFields := map[string][]string{}
	for _, className := range order {
		fields, err := packetFieldTypes(javap, jar, className)
		if err != nil {
			fail(fmt.Errorf("javap fields %s: %w", className, err))
		}
		packetFields[className] = fields
	}

	used := make([]bool, len(methods))
	ordered := make([]methodEntry, 0, len(methods))
	var unmapped []string

	for _, className := range order {
		base := strings.TrimSuffix(className, "CallPacket")
		base = strings.TrimSuffix(base, "2")
		base = strings.TrimSuffix(base, "3")
		base = strings.TrimSuffix(base, "4")
		base = strings.TrimSuffix(base, "5")
		if base == "" {
			continue
		}
		methodName := lowerFirst(base)
		fields := packetFields[className]
		candidateIdx := matchCandidates(methods, used, methodName, fields)
		if candidateIdx < 0 {
			unmapped = append(unmapped, className)
			continue
		}
		used[candidateIdx] = true
		ordered = append(ordered, methods[candidateIdx])
	}

	// append any leftover methods (should not happen)
	for i, m := range methods {
		if !used[i] {
			ordered = append(ordered, m)
		}
	}

	if err := writeOrderedCSV(csvOut, ordered); err != nil {
		fail(err)
	}

	if len(unmapped) > 0 {
		fmt.Fprintf(os.Stderr, "warning: unmapped packets: %s\n", strings.Join(unmapped, ", "))
	}
	fmt.Printf("wrote ordered csv: %s\n", csvOut)
}

func loadMethods(path string) ([]methodEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, errors.New("no rows")
	}

	var out []methodEntry
	for i, row := range rows[1:] {
		if len(row) < 3 {
			continue
		}
		sig := row[2]
		method, params := parseSignature(sig)
		if method == "" {
			continue
		}
		remote := row[1]
		calledServer := true
		if strings.Contains(remote, "called") {
			calledServer = strings.Contains(remote, "Loc.server")
		}
		if !calledServer && len(params) > 0 {
			params = params[1:]
		}
		out = append(out, methodEntry{
			Index:        i,
			File:         row[0],
			Remote:       remote,
			Signature:    sig,
			MethodName:   method,
			ParamTypes:   params,
			CalledServer: calledServer,
		})
	}
	return out, nil
}

func parseSignature(sig string) (string, []string) {
	sig = strings.TrimSpace(sig)
	re := regexp.MustCompile(`\bvoid\s+([a-zA-Z0-9_]+)\s*\((.*)\)`)
	m := re.FindStringSubmatch(sig)
	if len(m) < 3 {
		return "", nil
	}
	method := m[1]
	paramStr := strings.TrimSpace(m[2])
	if paramStr == "" {
		return method, nil
	}
	params := splitParams(paramStr)
	types := make([]string, 0, len(params))
	for _, p := range params {
		pt := paramType(p)
		if pt == "" {
			types = append(types, "")
			continue
		}
		types = append(types, normalizeType(pt))
	}
	return method, types
}

func splitParams(s string) []string {
	var out []string
	var buf strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(buf.String()))
				buf.Reset()
				continue
			}
		}
		buf.WriteByte(ch)
	}
	if buf.Len() > 0 {
		out = append(out, strings.TrimSpace(buf.String()))
	}
	return out
}

func paramType(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "final ")
	lastSpace := strings.LastIndex(p, " ")
	if lastSpace == -1 {
		return p
	}
	return strings.TrimSpace(p[:lastSpace])
}

func normalizeType(t string) string {
	t = strings.TrimSpace(t)
	t = strings.TrimSuffix(t, "...")
	if i := strings.Index(t, "<"); i >= 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(t)
	arraySuffix := ""
	for strings.HasSuffix(t, "[]") {
		arraySuffix += "[]"
		t = strings.TrimSuffix(t, "[]")
	}
	if idx := strings.LastIndex(t, "."); idx >= 0 {
		t = t[idx+1:]
	}
	return t + arraySuffix
}

func callPacketOrder(javap, jar string) ([]string, error) {
	out, err := exec.Command(javap, "-classpath", jar, "-v", "mindustry.gen.Call").Output()
	if err != nil {
		return nil, err
	}
	lines := bufio.NewScanner(bytes.NewReader(out))
	var order []string
	inBootstrap := false
	re := regexp.MustCompile(`REF_newInvokeSpecial\s+mindustry/gen/([A-Za-z0-9_]+)\.`)
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if strings.HasPrefix(line, "BootstrapMethods:") {
			inBootstrap = true
			continue
		}
		if !inBootstrap {
			continue
		}
		if strings.HasPrefix(line, "LineNumberTable:") {
			break
		}
		if strings.Contains(line, "REF_newInvokeSpecial mindustry/gen/") {
			m := re.FindStringSubmatch(line)
			if len(m) == 2 {
				order = append(order, m[1])
			}
		}
	}
	if err := lines.Err(); err != nil {
		return nil, err
	}
	if len(order) == 0 {
		return nil, errors.New("no packet order found")
	}
	return order, nil
}

func packetFieldTypes(javap, jar, className string) ([]string, error) {
	out, err := exec.Command(javap, "-classpath", jar, "-public", "mindustry.gen."+className).Output()
	if err != nil {
		return nil, err
	}
	var fields []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	re := regexp.MustCompile(`^public\s+(?:static\s+)?(?:final\s+)?([A-Za-z0-9_.$\\[\\]]+)\s+([A-Za-z0-9_]+);`)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.Contains(line, "(") || !strings.HasPrefix(line, "public ") {
			continue
		}
		m := re.FindStringSubmatch(line)
		if len(m) == 3 {
			t := normalizeType(m[1])
			fields = append(fields, t)
		}
	}
	return fields, sc.Err()
}

func matchCandidates(methods []methodEntry, used []bool, name string, fields []string) int {
	var fallback int = -1
	for i, m := range methods {
		if used[i] || m.MethodName != name {
			continue
		}
		if fallback == -1 {
			fallback = i
		}
		if len(m.ParamTypes) != len(fields) {
			continue
		}
		match := true
		for j := range fields {
			if m.ParamTypes[j] != fields[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return fallback
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func writeOrderedCSV(path string, rows []methodEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"file", "remote", "signature"}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{r.File, r.Remote, r.Signature}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
