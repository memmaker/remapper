package recfile

import (
	"os"
	"strings"
)

// PatchFieldInFile rewrites one field of some records in a rec file in place, leaving every other line as it was:
// comments, blank lines, section headers and fields it doesn't touch survive a save. A record is identified by
// the value of its key field (internal_name). For each record, values(id) returns the new values of field, or ok
// false to leave the record alone. The record's old field lines go, and the new ones take the place of the first
// of them, or follow the record's last field if it had none.
func PatchFieldInFile(path, key, field string, values func(id string) (newValues []string, ok bool)) error {
	return PatchTypedFieldInFile(path, key, field, func(_, id string) ([]string, bool) { return values(id) })
}

// PatchTypedFieldInFile is PatchFieldInFile for files with "%rec: <type>" sections: values also gets the record's
// type ("default" before the first %rec line), so equal ids in different sections stay apart. No new values
// removes the field.
func PatchTypedFieldInFile(path, key, field string, values func(recType, id string) (newValues []string, ok bool)) error {
	recType := "default"
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	var out []string
	var rec []string // the current record's lines, comments inside it included
	flush := func() {
		defer func() { rec = nil }()
		id := ""
		for _, l := range rec {
			if name, value, ok := fieldLine(l); ok && name == key {
				id = value
				break
			}
		}
		newValues, ok := values(recType, id)
		if id == "" || !ok {
			out = append(out, rec...)
			return
		}
		var kept []string
		insertAt, lastField := -1, -1
		for _, l := range rec {
			if name, _, ok := fieldLine(l); ok && name == field {
				if insertAt < 0 {
					insertAt = len(kept)
				}
				continue
			}
			if _, _, ok := fieldLine(l); ok {
				lastField = len(kept)
			}
			kept = append(kept, l)
		}
		if insertAt < 0 {
			insertAt = lastField + 1
		}
		var added []string
		for _, v := range newValues {
			added = append(added, field+": "+v)
		}
		out = append(out, kept[:insertAt]...)
		out = append(out, added...)
		out = append(out, kept[insertAt:]...)
	}
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "%rec:") {
			flush()
			if strings.HasPrefix(t, "%rec:") {
				recType = strings.TrimSpace(t[len("%rec:"):])
			}
			out = append(out, l)
			continue
		}
		rec = append(rec, l)
	}
	flush()
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}

// fieldLine reads "name: value"; comments and continuation lines ("+ ...") are not fields.
func fieldLine(l string) (name, value string, ok bool) {
	if strings.HasPrefix(l, "#") || strings.HasPrefix(l, "+") {
		return "", "", false
	}
	name, value, ok = strings.Cut(l, ":")
	if !ok || strings.ContainsAny(name, " \t") {
		return "", "", false
	}
	return name, strings.TrimSpace(value), true
}
