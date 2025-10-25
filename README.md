# tap

A minimal Go event logging library with non-blocking bus.

## Usage

```go
import "github.com/jon-ski/tap"

sink := func(e tap.Event) {
    log.Printf("%s: %s", e.Op, e.Msg)
}

bus := tap.NewBus(sink, 0)
bus.Emit(tap.Event{Kind: tap.EventInfo, Op: "example", Msg: "hello"})
bus.Close()
```

## License

Apache 2.0
