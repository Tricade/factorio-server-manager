package factorio

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

var ErrModPackMergeConflict = errors.New("mod pack cannot be added until conflicts are resolved")
var ErrModPackMergeStale = errors.New("the profile, game version or mod pack changed; review the pack again")
var ErrInvalidModPackMerge = errors.New("invalid mod pack merge request")

type ModPackMergeItem struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Enabled bool   `json:"enabled"`
}

type ModPackMergeConflict struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type ModPackMergePlan struct {
	Revision  string                 `json:"revision"`
	Additions []ModPackMergeItem     `json:"additions"`
	Kept      []ModPackMergeItem     `json:"kept"`
	Conflicts []ModPackMergeConflict `json:"conflicts"`
}

type mergeModSet struct {
	root    string
	files   []os.FileInfo
	mods    map[string]ModInfo
	list    map[string]json.RawMessage
	entries []json.RawMessage
	states  map[string]bool
}

var modPackMergeSource = func(name string) string {
	return filepath.Join(bootstrap.GetConfig().FactorioModPackDir, name)
}
var modPackMergeBuiltIn = func(name string) (builtInModInfo, bool, error) {
	return (&modDependencyPlanner{}).readBuiltInMod(name)
}

// Both operations own the exclusive profile gate, including pack reads. Pack
// editing and active-profile requests hold the shared gate in the API router.
func lockModPackMerge() (func(), error) {
	profileDataGate.Lock()
	serverLifecycleMutex.Lock()
	if GetFactorioServer().IsBusy() {
		serverLifecycleMutex.Unlock()
		profileDataGate.Unlock()
		return nil, ErrServerActive
	}
	return func() { serverLifecycleMutex.Unlock(); profileDataGate.Unlock() }, nil
}

func PreviewModPackMerge(name string) (ModPackMergePlan, error) {
	unlock, err := lockModPackMerge()
	if err != nil {
		return ModPackMergePlan{}, err
	}
	defer unlock()
	plan, _, _, err := planModPackMerge(name)
	return plan, err
}

func MergeModPack(name, revision string) (ModPackMergePlan, error) {
	if len(revision) != sha256.Size*2 {
		return ModPackMergePlan{}, ErrInvalidModPackMerge
	}
	if _, err := hex.DecodeString(revision); err != nil {
		return ModPackMergePlan{}, ErrInvalidModPackMerge
	}
	unlock, err := lockModPackMerge()
	if err != nil {
		return ModPackMergePlan{}, err
	}
	defer unlock()
	plan, active, pack, err := planModPackMerge(name)
	if err != nil {
		return plan, err
	}
	if revision != plan.Revision {
		return plan, ErrModPackMergeStale
	}
	if len(plan.Conflicts) > 0 {
		return plan, ErrModPackMergeConflict
	}
	if len(plan.Additions) == 0 {
		return plan, nil
	}

	swap, err := newDirectoryEntrySwap(active.root, ".mods-merge-staging-", ".mods-merge-backup-")
	if err != nil {
		return plan, err
	}
	defer swap.cleanup()
	// The snapshot was read before creating the child staging directories, so
	// copying the active entries cannot recursively copy staging into itself.
	for _, info := range active.files {
		if err := copyProfileFile(filepath.Join(active.root, info.Name()), filepath.Join(swap.staging, info.Name()), info); err != nil {
			return plan, err
		}
	}
	for _, item := range plan.Additions {
		archive := pack.mods[item.Name].FileName
		info, err := os.Lstat(filepath.Join(pack.root, archive))
		if err != nil || !info.Mode().IsRegular() {
			return plan, errors.New("mod pack archive is no longer a regular file")
		}
		if err := copyProfileFile(filepath.Join(pack.root, archive), filepath.Join(swap.staging, archive), info); err != nil {
			return plan, err
		}
	}
	list, err := mergedModList(active, pack, plan.Additions)
	if err != nil {
		return plan, err
	}
	if err := writeFileAtomically(filepath.Join(swap.staging, "mod-list.json"), list, 0600); err != nil {
		return plan, err
	}
	if _, err := readMergeModSet(swap.staging); err != nil {
		return plan, err
	}
	if err := swap.activate(); err != nil {
		return plan, err
	}
	if _, err := NewMods(active.root); err != nil {
		return plan, errors.Join(err, swap.rollback())
	}
	if err := swap.commit(); err != nil {
		log.Printf("Mod pack merged; backup cleanup failed: %v", err)
	}
	return plan, nil
}

func planModPackMerge(name string) (ModPackMergePlan, mergeModSet, mergeModSet, error) {
	plan := ModPackMergePlan{Additions: []ModPackMergeItem{}, Kept: []ModPackMergeItem{}, Conflicts: []ModPackMergeConflict{}}
	var active, pack mergeModSet
	if ValidatePathElement(name) != nil {
		return plan, active, pack, ErrInvalidModPackMerge
	}
	profile, err := activeMapSnapshotProfile()
	if err != nil {
		return plan, active, pack, err
	}
	active, err = readMergeModSet(profileActiveDirectories()["mods"])
	if err != nil {
		return plan, active, pack, err
	}
	pack, err = readMergeModSet(modPackMergeSource(name))
	if err != nil {
		return plan, active, pack, err
	}
	version := GetFactorioServer().Snapshot().Version
	digest := sha256.New()
	for _, value := range []string{profile.ID, version.String(), name} {
		writeModStartupHashPart(digest, []byte(value))
	}
	visited := 0
	var totalBytes uint64
	for _, set := range []mergeModSet{active, pack} {
		writeModStartupHashPart(digest, []byte(set.root))
		for _, info := range set.files {
			if info.Size() < 0 || uint64(info.Size()) > backupMaxExpandedBytes-totalBytes {
				return plan, active, pack, errors.New("combined mod directories exceed the 16 GiB inspection limit")
			}
			totalBytes += uint64(info.Size())
			if err := hashModStartupEntryMetadata(digest, set.root, filepath.Join(set.root, info.Name()), &visited); err != nil {
				return plan, active, pack, err
			}
		}
	}
	plan.Revision = hex.EncodeToString(digest.Sum(nil))
	conflict := func(name, reason string) {
		plan.Conflicts = append(plan.Conflicts, ModPackMergeConflict{Name: name, Reason: reason})
	}
	combined := make(map[string]ModInfo)
	states := make(map[string]bool)
	filenames := make(map[string]bool)
	for _, info := range active.files {
		filenames[strings.ToLower(info.Name())] = true
	}
	for name, info := range active.mods {
		combined[name] = info
	}
	for name, enabled := range active.states {
		states[name] = enabled
	}
	names := make([]string, 0, len(pack.mods))
	for name := range pack.mods {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		incoming := pack.mods[name]
		if current, ok := active.mods[name]; ok {
			plan.Kept = append(plan.Kept, ModPackMergeItem{Name: name, Version: current.Version, Enabled: active.states[name]})
			if current.Version != incoming.Version {
				conflict(name, fmt.Sprintf("Installed version %s differs from pack version %s. Keep the installed version by removing this mod from the pack, or align the versions first.", current.Version, incoming.Version))
			}
			continue
		}
		if filenames[strings.ToLower(incoming.FileName)] {
			conflict(name, "The archive filename is already used by the active profile.")
			continue
		}
		filenames[strings.ToLower(incoming.FileName)] = true
		enabled, hasState := states[name]
		if !hasState {
			enabled = pack.states[name]
			states[name] = enabled
		}
		combined[name] = incoming
		plan.Additions = append(plan.Additions, ModPackMergeItem{Name: name, Version: incoming.Version, Enabled: enabled})
	}
	if version == NilVersion {
		conflict("base", "Install a Factorio version before adding a pack.")
	}
	if !states["base"] {
		conflict("base", "The base mod must be enabled.")
	}
	versions := map[string]Version{"base": version, "core": version}
	states["core"] = true
	for name, info := range combined {
		var parsed Version
		if err := parsed.UnmarshalText([]byte(info.Version)); err != nil {
			return plan, active, pack, fmt.Errorf("invalid mod version for %s", name)
		}
		versions[name] = parsed
		if states[name] && !version.SameFactorioLine(info.FactorioVersion) {
			conflict(name, "This mod targets Factorio "+info.FactorioVersion.FactorioLine()+", not "+version.FactorioLine()+".")
		}
	}
	for _, name := range spaceAgeFeatureMods {
		if !states[name] {
			continue
		}
		info, found, err := modPackMergeBuiltIn(name)
		if err != nil {
			return plan, active, pack, err
		}
		if !found {
			conflict(name, "The enabled expansion component is not installed.")
			continue
		}
		versions[name] = info.Version
		combined[name] = ModInfo{Name: name, Dependencies: info.Dependencies}
	}
	// A remembered version pin can exist even after its archive was removed.
	// Never activate a newly added archive against a different preserved pin.
	for _, raw := range active.entries {
		var entry struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		_ = json.Unmarshal(raw, &entry)
		if candidate, found := versions[entry.Name]; found && entry.Version != "" && !isMergeBuiltIn(entry.Name) {
			var pinned Version
			if pinned.UnmarshalText([]byte(entry.Version)) != nil || !candidate.Equals(pinned) {
				conflict(entry.Name, "The active mod list pins a different version. Update or remove that pin before adding this archive.")
			}
		}
	}
	names = names[:0]
	for name := range combined {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !states[name] {
			continue
		}
		for _, raw := range combined[name].Dependencies {
			dependency, err := ParseModDependency(raw)
			if err != nil {
				conflict(name, "Invalid dependency declaration: "+raw)
				continue
			}
			dependencyVersion, present := versions[dependency.Name]
			present = present && states[dependency.Name]
			matches := present && matchesAllConstraints(dependencyVersion, []ParsedModDependency{dependency})
			if dependency.Kind == ModDependencyIncompatible {
				if matches {
					conflict(name, "Incompatible enabled mod: "+dependency.Raw)
				}
			} else if dependency.Kind == ModDependencyRequired {
				if !matches {
					conflict(name, "Required dependency is missing, disabled or has the wrong version: "+dependency.Raw)
				}
			} else if present && !matches {
				conflict(name, "Installed optional dependency has the wrong version: "+dependency.Raw)
			}
		}
	}
	return plan, active, pack, nil
}

func readMergeModSet(root string) (mergeModSet, error) {
	set := mergeModSet{root: root, mods: map[string]ModInfo{}, list: map[string]json.RawMessage{}, states: map[string]bool{"base": true}}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return set, errors.New("mod directory is unavailable or unsafe")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return set, err
	}
	if len(entries) > 4096 {
		return set, errors.New("mod directory contains too many entries")
	}
	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(root, entry.Name()))
		if err != nil {
			return set, err
		}
		if ValidatePathElement(entry.Name()) != nil || !info.Mode().IsRegular() {
			return set, errors.New("additive loading requires regular mod ZIPs; unpacked directories and links are not supported")
		}
		if info.Size() > maximumModPortalArchiveBytes {
			return set, errors.New("mod entry exceeds the supported size")
		}
		set.files = append(set.files, info)
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".zip") {
			continue
		}
		metadata, err := readMergeModInfo(filepath.Join(root, entry.Name()))
		if err != nil {
			return set, fmt.Errorf("invalid mod archive %s: %w", entry.Name(), err)
		}
		if _, exists := set.mods[metadata.Name]; exists {
			return set, fmt.Errorf("multiple versions of %s are present; remove unused versions before adding a pack", metadata.Name)
		}
		metadata.FileName = entry.Name()
		set.mods[metadata.Name] = metadata
	}
	data, err := readBoundedRegularFile(filepath.Join(root, "mod-list.json"), maximumModStartupModListBytes, true)
	if err != nil {
		return set, err
	}
	if data == nil {
		set.entries = append(set.entries, json.RawMessage(`{"name":"base","enabled":true}`))
		names := make([]string, 0, len(set.mods))
		for name := range set.mods {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			data, _ := json.Marshal(ModSimple{Name: name, Enabled: true})
			set.entries = append(set.entries, data)
			set.states[name] = true
		}
		return set, nil
	}
	if json.Unmarshal(data, &set.list) != nil || set.list == nil || json.Unmarshal(set.list["mods"], &set.entries) != nil || set.entries == nil || len(set.entries) > 4096 {
		return set, errors.New("invalid mod-list.json")
	}
	seen := make(map[string]bool)
	for _, entry := range set.entries {
		var value struct {
			Name    string `json:"name"`
			Enabled *bool  `json:"enabled"`
			Version string `json:"version"`
		}
		if json.Unmarshal(entry, &value) != nil || ValidatePathElement(value.Name) != nil || value.Enabled == nil || seen[value.Name] {
			return set, errors.New("invalid or duplicate mod-list entry")
		}
		if mod, found := set.mods[value.Name]; found && value.Version != "" {
			var pinned, installed Version
			if pinned.UnmarshalText([]byte(value.Version)) != nil || installed.UnmarshalText([]byte(mod.Version)) != nil || !pinned.Equals(installed) {
				return set, fmt.Errorf("mod-list version pin does not match the archive for %s", value.Name)
			}
		}
		seen[value.Name] = true
		set.states[value.Name] = *value.Enabled
	}
	return set, nil
}

func readMergeModInfo(path string) (ModInfo, error) {
	var info ModInfo
	reader, err := zip.OpenReader(path)
	if err != nil {
		return info, err
	}
	defer reader.Close()
	if err := inspectBackupZip(&reader.Reader, nil); err != nil {
		return info, err
	}
	count := 0
	for _, file := range reader.File {
		if len(strings.Split(file.Name, "/")) != 2 || !strings.HasSuffix(file.Name, "/info.json") {
			continue
		}
		count++
		if file.UncompressedSize64 > 1<<20 {
			return info, errors.New("mod metadata is too large")
		}
		input, err := file.Open()
		if err != nil {
			return info, err
		}
		data, readErr := io.ReadAll(io.LimitReader(input, (1<<20)+1))
		input.Close()
		if readErr != nil || len(data) > 1<<20 || json.Unmarshal(data, &info) != nil {
			return info, errors.New("invalid mod metadata")
		}
	}
	if count != 1 || ValidatePathElement(info.Name) != nil || ValidatePathElement(info.Version) != nil || isMergeBuiltIn(info.Name) || len(info.Dependencies) > 1024 {
		return info, errors.New("invalid mod identity")
	}
	for _, dependency := range info.Dependencies {
		if len(dependency) > 1024 {
			return info, errors.New("dependency declaration is too long")
		}
	}
	return info, nil
}

func isMergeBuiltIn(name string) bool {
	return name == "base" || name == "core" || name == "space-age" || name == "quality" || name == "elevated-rails"
}

func mergedModList(active, pack mergeModSet, additions []ModPackMergeItem) ([]byte, error) {
	for _, item := range additions {
		if _, exists := active.states[item.Name]; exists {
			continue
		}
		var raw json.RawMessage
		for _, entry := range pack.entries {
			var mod ModSimple
			if json.Unmarshal(entry, &mod) == nil && mod.Name == item.Name {
				raw = entry
				break
			}
		}
		if raw == nil {
			raw, _ = json.Marshal(ModSimple{Name: item.Name, Enabled: item.Enabled})
		}
		active.entries = append(active.entries, raw)
	}
	mods, err := json.Marshal(active.entries)
	if err != nil {
		return nil, err
	}
	active.list["mods"] = mods
	contents, err := json.MarshalIndent(active.list, "", "  ")
	return append(contents, '\n'), err
}
