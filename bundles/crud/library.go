// Package crud provides a copyable, policy-free CRUD Flow bundle.
package crud

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sync"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/action"
	"github.com/nexssp/kernel/xfs"
	"gopkg.in/yaml.v3"
)

const ID = "crud"

var (
	ErrConflict = errors.New("crud: conflict")
	ErrNotFound = errors.New("crud: not found")
)

//go:embed crud.nflow
var FS embed.FS

// Snapshot is a read view and opaque token supplied back to the Store on patch.
type Snapshot struct {
	Entries map[string]any
	Token   any
}

// Patch describes a single-key mapping mutation.
type Patch struct {
	Key    string
	Remove bool
	Value  any
}

// Store is the persistence boundary; policy remains in NFlow.
type Store interface {
	Read(ctx context.Context, path, section string) (Snapshot, error)
	Patch(ctx context.Context, path, section string, snapshot Snapshot, patch Patch) error
}

type storeKey struct{}

// WithStore scopes a Store override to Execute or an external Flow runner context.
func WithStore(ctx context.Context, store Store) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, storeKey{}, store)
}

// YAMLStore preserves unrelated YAML nodes and writes patches atomically.
type YAMLStore struct{}

func init() {
	compiler.Register(ID, Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/crud/nexssflow", Bundle)
}

func Bundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{ID: ID, Libraries: []action.Library{{Name: ID, Actions: []action.AnyAction{
		action.New("crud.read", readAction).Description("Read a mapping and return its source token").Build(),
		action.New("crud.patch", patchAction).Description("Apply a single-key mapping patch").Build(),
		action.New("crud.reject", reject).Description("Translate a policy rejection").Build(),
	}}}}
}
func Loader() compiler.SourceLoader { return sourceLoader{} }

type sourceLoader struct{}

func (sourceLoader) Load(_, target string) ([]byte, string, error) {
	if filepath.Base(target) != "crud.nflow" {
		return nil, "", fmt.Errorf("crud: unknown source %q", target)
	}
	b, e := FS.ReadFile("crud.nflow")
	return b, "crud.nflow", e
}

const yamlStringTag = "!!str"

var executionConfig = sync.OnceValues(func() (runner.Config, error) { return runner.BuildConfig(append(native.Bundles(), Bundle(nil))) })

//nolint:contextcheck // A nil context has no parent; retain the API's background fallback.
func Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg, err := executionConfig()
	if err != nil {
		return nil, err
	}
	ctx = compiler.WithSourceLoader(ctx, Loader())
	ex, err := runner.Execute(ctx, cfg, "@include \"crud.nflow\"\ncrud.pipeline.crud", "crud-entry.nflow", input)
	if err != nil {
		return nil, err
	}
	out, ok := ex.Output.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("crud: expected result object, got %T", ex.Output)
	}
	return out, nil
}

func storeFrom(ctx context.Context) Store {
	if s, ok := ctx.Value(storeKey{}).(Store); ok && s != nil {
		return s
	}
	return YAMLStore{}
}

func readAction(ctx context.Context, in map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, pok := in["path"].(string)
	sec, sok := in["section"].(string)
	if !pok || p == "" || !sok {
		return nil, errors.New("crud.read: invalid path or section")
	}
	snap, err := storeFrom(ctx).Read(ctx, p, sec)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if snap.Entries == nil {
		snap.Entries = map[string]any{}
	}
	out := clone(in)
	out["entries"] = snap.Entries
	out["source"] = snap.Token
	return out, nil
}

func patchAction(ctx context.Context, in map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, pok := in["path"].(string)
	sec, sok := in["section"].(string)
	token, tok := in["source"]
	m, mok := in["patch"].(map[string]any)
	key, kok := m["key"].(string)
	remove, rok := m["remove"].(bool)
	if !pok || p == "" || !sok || !tok || !mok || !kok || !rok {
		return nil, errors.New("crud.patch: invalid patch envelope")
	}
	snap := Snapshot{Token: token}
	if entries, ok := in["entries"].(map[string]any); ok {
		snap.Entries = entries
	}
	if err := storeFrom(ctx).Patch(ctx, p, sec, snap, Patch{Key: key, Remove: remove, Value: m["value"]}); err != nil {
		return nil, err
	}
	return map[string]any{"name": key}, nil
}

func reject(_ context.Context, in map[string]any) (map[string]any, error) {
	message, _ := in["message"].(string)
	switch fmt.Sprint(in["code"]) {
	case "conflict":
		return nil, fmt.Errorf("%w: %s", ErrConflict, message)
	case "not_found":
		return nil, fmt.Errorf("%w: %s", ErrNotFound, message)
	default:
		return nil, fmt.Errorf("crud: %s", message)
	}
}

func clone(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+2)
	maps.Copy(out, in)
	return out
}

func (YAMLStore) Read(ctx context.Context, path, section string) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return Snapshot{}, fmt.Errorf("crud: read: %w", err)
	}
	doc, err := decodeDocument(data)
	if err != nil {
		return Snapshot{}, err
	}
	entries := map[string]any{}
	if node := mappingValue(doc.Content[0], section); node != nil && node.Tag != "!!null" {
		if node.Kind != yaml.MappingNode {
			return Snapshot{}, fmt.Errorf("crud: %s must be a mapping", section)
		}
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Kind != yaml.ScalarNode || node.Content[i].Tag != yamlStringTag {
				return Snapshot{}, errors.New("crud: section keys must be strings")
			}
		}
		if err := node.Decode(&entries); err != nil {
			return Snapshot{}, fmt.Errorf("crud: decode section: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Entries: entries, Token: string(data)}, nil
}

func (YAMLStore) Patch(ctx context.Context, path, section string, snap Snapshot, patch Patch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source, ok := snap.Token.(string)
	if !ok {
		return errors.New("crud: YAML source token must be a string")
	}
	doc, err := decodeDocument([]byte(source))
	if err != nil {
		return err
	}
	root := doc.Content[0]
	sectionNode := mappingValue(root, section)
	if sectionNode == nil || sectionNode.Tag == "!!null" {
		sectionNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setMapping(root, section, sectionNode)
	}
	if sectionNode.Kind != yaml.MappingNode {
		return errors.New("crud: section must be a mapping")
	}
	if patch.Remove {
		removeMapping(sectionNode, patch.Key)
	} else {
		value, encodeErr := encodeValue(patch.Value)
		if encodeErr != nil {
			return encodeErr
		}
		setMapping(sectionNode, patch.Key, value)
	}
	data, err := yaml.Marshal(doc.Content[0])
	if err != nil {
		return fmt.Errorf("crud: marshal: %w", err)
	}
	if _, err := decodeDocument(data); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("crud: create directory: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := xfs.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("crud: write: %w", err)
	}
	return nil
}

func encodeValue(value any) (node *yaml.Node, err error) {
	defer func() {
		if p := recover(); p != nil {
			node = nil
			err = fmt.Errorf("crud: value not serializable: %v", p)
		}
	}()
	node = &yaml.Node{}
	if err = node.Encode(value); err != nil {
		return nil, fmt.Errorf("crud: encode value: %w", err)
	}
	return node, nil
}

func decodeDocument(data []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("crud: malformed YAML: %w", err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("crud: multiple or malformed YAML documents are not supported")
	}
	if len(doc.Content) == 0 || doc.Content[0].Tag == "!!null" {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("crud: document must be a mapping")
	}
	var check any
	if err := doc.Decode(&check); err != nil {
		return nil, fmt.Errorf("crud: invalid YAML mapping: %w", err)
	}
	return &doc, nil
}

func mappingValue(n *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Tag == yamlStringTag && n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func setMapping(n *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Tag == yamlStringTag && n.Content[i].Value == key {
			n.Content[i+1] = value
			return
		}
	}
	n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func removeMapping(n *yaml.Node, key string) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Tag == yamlStringTag && n.Content[i].Value == key {
			n.Content = append(n.Content[:i], n.Content[i+2:]...)
			return
		}
	}
}
