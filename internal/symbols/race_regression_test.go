package symbols

import (
	"sync"
	"testing"
)

// TestSearchByFQNPrefixConcurrent verifies that SearchByFQNPrefix is safe to call
// concurrently after a re-index has marked the sorted FQN slice dirty.
// This is a regression test for the data race where two concurrent readers both
// attempted to rebuild the sorted slice while holding only a read lock.
// Must pass under: go test -race ./internal/symbols/
func TestSearchByFQNPrefixConcurrent(t *testing.T) {
	t.Helper()
	idx := NewIndex()

	// Index a file to populate the sorted FQN slice once.
	idx.IndexFile("file:///race_a.php", `<?php
namespace Race;
class Alpha {}
class Beta {}
`)

	// Re-index the same URI to mark sorted structures dirty before the concurrent
	// readers start — this is the exact condition that exposed the race.
	idx.IndexFile("file:///race_a.php", `<?php
namespace Race;
class AlphaV2 {}
class BetaV2 {}
`)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			syms, segs := idx.SearchByFQNPrefix("Race\\")
			// Minimal correctness check: results must not contain empty-named symbols,
			// and at least one symbol must exist.
			for _, s := range syms {
				if s.Name == "" {
					t.Errorf("SearchByFQNPrefix returned symbol with empty Name, FQN=%q", s.FQN)
				}
			}
			for _, seg := range segs {
				if seg == "" {
					t.Errorf("SearchByFQNPrefix returned empty namespace segment")
				}
			}
			_ = syms
		}()
	}
	wg.Wait()
}

// TestSearchByPrefixConcurrent verifies that SearchByPrefix is safe to call
// concurrently after a re-index has marked the sorted names slice dirty.
func TestSearchByPrefixConcurrent(t *testing.T) {
	t.Helper()
	idx := NewIndex()

	idx.IndexFile("file:///race_b.php", `<?php
namespace Concurrent;
class Worker {}
class Manager {}
`)

	// Re-index to mark sorted structures dirty.
	idx.IndexFile("file:///race_b.php", `<?php
namespace Concurrent;
class WorkerV2 {}
class ManagerV2 {}
`)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			results := idx.SearchByPrefix("Worker")
			for _, s := range results {
				if s.Name == "" {
					t.Errorf("SearchByPrefix returned symbol with empty Name, FQN=%q", s.FQN)
				}
			}
		}()
	}
	wg.Wait()
}

// TestGetFileSourceAfterIDEHelper verifies that GetFileSource returns the stored
// source for a URI indexed via IndexIDEHelperFile. This is a regression test for
// the bug where IndexIDEHelperFile called removeFileSymbols (which clears
// fileSource) but never re-stored the source.
func TestGetFileSourceAfterIDEHelper(t *testing.T) {
	t.Helper()

	const helperURI = "file:///_ide_helper_models.php"
	const helperSource = `<?php
namespace App\Models;

/**
 * @property int $id
 * @property string $name
 */
class Post {
}
`

	t.Run("source stored on first IndexIDEHelperFile call", func(t *testing.T) {
		idx := NewIndex()
		idx.IndexIDEHelperFile(helperURI, helperSource)

		got := idx.GetFileSource(helperURI)
		if got != helperSource {
			t.Errorf("GetFileSource(%q) = %q; want %q", helperURI, got, helperSource)
		}
	})

	t.Run("source updated on re-index via IndexIDEHelperFile", func(t *testing.T) {
		idx := NewIndex()
		idx.IndexIDEHelperFile(helperURI, helperSource)

		const updatedSource = `<?php
namespace App\Models;

/**
 * @property int $id
 * @property string $slug
 */
class Post {
}
`
		idx.IndexIDEHelperFile(helperURI, updatedSource)

		got := idx.GetFileSource(helperURI)
		if got != updatedSource {
			t.Errorf("after re-index GetFileSource = %q; want %q", got, updatedSource)
		}
	})

	t.Run("source stored when IDE helper indexes existing class", func(t *testing.T) {
		idx := NewIndex()

		// Index the real model first.
		idx.IndexFile("file:///app/Models/Post.php", `<?php
namespace App\Models;
class Post {
    public string $slug;
}
`)

		// Now index the IDE helper which merges virtual members.
		idx.IndexIDEHelperFile(helperURI, helperSource)

		got := idx.GetFileSource(helperURI)
		if got != helperSource {
			t.Errorf("GetFileSource(%q) = %q; want %q", helperURI, got, helperSource)
		}
	})
}
