package card

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Vocab is the shared, venue-agnostic vocabulary from config/vocab.yaml.
// Both fetchers map into it. A value not listed is stored as empty.
type Vocab struct {
	subjects, deciders, units map[string]bool
}

type vocabFile struct {
	Subjects    []string `yaml:"subjects"`
	Deciders    []string `yaml:"deciders"`
	CutoffUnits []string `yaml:"cutoff_units"`
}

// LoadVocab reads a vocabulary file.
func LoadVocab(path string) (*Vocab, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseVocab(b)
}

// ParseVocab parses vocabulary YAML.
func ParseVocab(b []byte) (*Vocab, error) {
	var f vocabFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("vocab: %w", err)
	}
	return &Vocab{subjects: set(f.Subjects), deciders: set(f.Deciders), units: set(f.CutoffUnits)}, nil
}

func set(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func lookup(m map[string]bool, s string) Opt[string] {
	if m[s] {
		return Some(s)
	}
	return None[string]()
}

// Subject returns Some(s) if s is a listed subject, else empty.
func (v *Vocab) Subject(s string) Opt[string] { return lookup(v.subjects, s) }

// Decider returns Some(s) if s is a listed decider, else empty.
func (v *Vocab) Decider(s string) Opt[string] { return lookup(v.deciders, s) }

// CutoffUnit returns Some(s) if s is a listed cutoff unit, else empty.
func (v *Vocab) CutoffUnit(s string) Opt[string] { return lookup(v.units, s) }
