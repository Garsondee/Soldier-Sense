# 0001 - Game behaviour needs fast, weak random number generation, 2026-02-28

## Issue
In-game behaviour requires random number generation for sprite movement, AI decision-making, and other simulation elements. The decision was made to prioritize execution speed over cryptographic quality randomness, since game behaviour does not require unpredictable outputs suitable for security-sensitive operations.

## Decision
Use fast but weak (non-cryptographic) random number generators for all in-game asset behaviour and AI decisions. Ignore warnings about weak randomness from linters and security tools, as they are not applicable to this use case.

## Rationale
Game simulations place a premium on performance and speed. Sprites, particle effects, NPC behaviour, and tactical decisions are generated frequently and in high volumes. Cryptographic-quality randomness is unnecessary for these gameplay systems—only sufficient statistical distribution is required.

Weak random number generators (such as xorshift variants or PCG-light algorithms) provide:
- O(1) constant-time generation
- Minimal memory footprint
- Sufficient distribution for believable game behaviour
- Negligible risk since the output is not used for security or reproducibility requiring unpredictability

## Consequences
- ✓ Improved performance in tight simulation loops
- ✓ Reduced CPU overhead for high-frequency random calls
- ⚠ Security linters may flag weak RNG usage; these warnings should be suppressed in code review
- ⚠ Output is predictable to someone with knowledge of the algorithm and seed; this is acceptable for gameplay

## Implementation Notes
Developers should use standard fast RNG libraries available in their language without concern for security warnings. In Go, this might include simple implementations like `math/rand` for gameplay, suppressing any `gosec` warnings about weak randomness as appropriate.
