package app

import (
	"testing"

	"github.com/beriholic/segra/internal/download"
)

func TestChunkBlocksFillInByteOrder(t *testing.T) {
	// Ten bytes a cell, so a segment that stops mid-cell puts exactly one
	// cell on the frontier.
	n := chunkRows * chunkCols
	size := int64(10 * n)
	chunk := size / 4
	segs := []download.SegmentProgress{
		{Start: 0, Done: chunk, Total: chunk},
		{Start: chunk, Done: chunk, Total: chunk},
		{Start: 2 * chunk, Done: 135, Total: chunk},
		{Start: 3 * chunk, Done: 0, Total: chunk},
	}
	cells := chunkBlocks(download.Task{Size: size, Segments: segs})
	if len(cells) != n {
		t.Fatalf("cells = %d, want %d", len(cells), n)
	}
	// The first two segments landed, and so did the first 130 bytes of the
	// third: every cell up to byte 610 is whole.
	done := int(2*chunk+130) / 10
	for i := 0; i < done; i++ {
		if cells[i] != blockDone {
			t.Fatalf("cell %d = %d, want done", i, cells[i])
		}
	}
	// That cell covers bytes 610 to 619; the third segment stops at 615.
	if cells[done] != blockActive {
		t.Fatalf("cell %d = %d, want active on the frontier", done, cells[done])
	}
	for i := done + 1; i < n; i++ {
		if cells[i] != blockPending {
			t.Fatalf("cell %d = %d, want pending", i, cells[i])
		}
	}
}

func TestChunkBlocksCompletedWithoutRangesLightsEveryCell(t *testing.T) {
	cells := chunkBlocks(download.Task{Status: download.StatusCompleted, Size: 4096})
	if len(cells) != chunkRows*chunkCols {
		t.Fatalf("cells = %d, want %d", len(cells), chunkRows*chunkCols)
	}
	for i, state := range cells {
		if state != blockDone {
			t.Fatalf("cell %d = %d, want done", i, state)
		}
	}
}

func TestChunkBlocksNothingKnown(t *testing.T) {
	if got := chunkBlocks(download.Task{Status: download.StatusQueued, Size: 4096}); got != nil {
		t.Fatalf("cells = %v, want nil", got)
	}
}

func TestChunkBlocksPausedFrontierCools(t *testing.T) {
	if got := blockColor(blockActive, false); got != colBlockHot {
		t.Fatalf("running frontier = %v, want the hot colour", got)
	}
	if got := blockColor(blockActive, true); got != colBlockCold {
		t.Fatalf("paused frontier = %v, want the cold colour", got)
	}
}
