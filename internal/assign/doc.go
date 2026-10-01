// Package assign implements the Secret Santa drawer's randomized
// assignment solver: given a member list, pairwise exclusions, and
// directional history from prior draws, it computes a valid
// gifter→giftee mapping or returns a clear error when none exists. It
// performs no I/O and has no dependency on internal/db or internal/api;
// callers convert to and from their own ID types.
package assign
