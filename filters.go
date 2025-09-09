package canbus

// Typed and composable helpers for FrameFilter.

// ByID returns a filter that matches frames with the exact identifier.
func ByID(id uint32) FrameFilter {
    return func(f Frame) bool { return f.ID == id }
}

// ByIDs returns a filter that matches any of the provided identifiers.
func ByIDs(ids ...uint32) FrameFilter {
    // Build a small set for O(1) lookup.
    m := make(map[uint32]struct{}, len(ids))
    for _, id := range ids {
        m[id] = struct{}{}
    }
    return func(f Frame) bool {
        _, ok := m[f.ID]
        return ok
    }
}

// ByRange matches frames whose ID is within [minID, maxID], inclusive.
func ByRange(minID, maxID uint32) FrameFilter {
    if maxID < minID {
        // swap defensively
        minID, maxID = maxID, minID
    }
    return func(f Frame) bool { return f.ID >= minID && f.ID <= maxID }
}

// ByMask matches when (frame.ID & mask) == (id & mask).
func ByMask(id uint32, mask uint32) FrameFilter {
    want := id & mask
    return func(f Frame) bool { return (f.ID & mask) == want }
}

// StandardOnly matches standard (11-bit) identifiers.
func StandardOnly() FrameFilter {
    return func(f Frame) bool { return !f.Extended }
}

// ExtendedOnly matches extended (29-bit) identifiers.
func ExtendedOnly() FrameFilter {
    return func(f Frame) bool { return f.Extended }
}

// DataOnly matches non-RTR frames.
func DataOnly() FrameFilter {
    return func(f Frame) bool { return !f.RTR }
}

// RTROnly matches remote transmission request frames.
func RTROnly() FrameFilter {
    return func(f Frame) bool { return f.RTR }
}

// LenAtMost matches frames with data length <= n.
func LenAtMost(n uint8) FrameFilter {
    return func(f Frame) bool { return f.Len <= n }
}

// LenExactly matches frames with data length == n.
func LenExactly(n uint8) FrameFilter {
    return func(f Frame) bool { return f.Len == n }
}

// And composes two filters; the result matches when both match.
func And(a, b FrameFilter) FrameFilter {
    switch {
    case a == nil:
        return b
    case b == nil:
        return a
    default:
        return func(f Frame) bool { return a(f) && b(f) }
    }
}

// Or composes two filters; the result matches when either matches.
func Or(a, b FrameFilter) FrameFilter {
    switch {
    case a == nil:
        return b
    case b == nil:
        return a
    default:
        return func(f Frame) bool { return a(f) || b(f) }
    }
}

// Not inverts a filter.
func Not(a FrameFilter) FrameFilter {
    if a == nil {
        return func(f Frame) bool { return true }
    }
    return func(f Frame) bool { return !a(f) }
}


