# ADR 0001: Runtime Mailbox Carries Typed Values, Not Callbacks

Status: Accepted

## Context

The first tick-ownership migration correctly moved gameplay mutations off network goroutines, but the mailbox initially carried `func()` callbacks. Packet handlers therefore created capturing closures for movement, chat, block interaction, and lifecycle work.

That design was functionally correct but inappropriate for a high-frequency server hot path: closures may escape to the heap, they hide the exact kinds of work entering the runtime, and they would make it too easy for future systems or plugins to bypass ownership rules by scheduling arbitrary code.

## Decision

- `internal/runtime/mailbox` is a generic multi-producer/single-consumer **value** queue.
- The server owns a private `runtimeEvent` union with explicit event kinds for every accepted runtime transition.
- Network/protocol goroutines only decode data and enqueue a value event.
- The tick owner consumes events and performs the authoritative mutation through one explicit dispatcher.
- Ordinary events remain bounded; queue pressure disconnects the producing player rather than dropping authoritative work.
- Critical lifecycle values remain retained and are consumed before ordinary work.
- The movement handoff path is protected by a zero-allocation regression test.

## Consequences

Adding a new runtime mutation now requires an explicit event kind and an owner-side handler. This is intentional friction: reviewers can enumerate every cross-thread mutation boundary by reading the event definition and dispatcher.

The public Plugin API and scheduler must not expose this internal event type, `PlayerSession`, protocol packets, or a raw "run arbitrary callback on the tick thread" escape hatch. Plugin scheduling will use a separate public contract that is translated into controlled runtime work by the core.

Callbacks are still valid inside owner-only code where they are not a cross-thread scheduling primitive (for example, local iteration helpers). This ADR only prohibits callback values as the runtime mailbox protocol.
