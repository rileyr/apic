# apic

api clients, because who has time for that? this is a little library to gsd

## http

fire up an http client real easy like:
```golang
package main

import(
    "fmt"
    "log/slog"

    "https://github.com/rileyr/apic"
)

func main() {
    client := apic.NewHTTPClient("my-cool-api-root",
        apic.WithLogger(slog.Default()),
        apic.WithBefore(addAuthSignature),
    )

    type request struct{
        Foo string
        Bar struct {
            Biz int
        }
    }

    type response struct {
        Id   int
        Whiz string
    }

    // blah blah imagine these are filled in:
    var (
        req request
        rsp response
    )

    if err := client.Post("/some/path", req, &rsp); err != nil{
        panic("OH MY GOSH " + err.Error())
    }

    fmt.Printf("Got a response!:\n%#v\n", rsp)
}

func addAuthSignature(req *http.Request) error {
    req.Header.Add("AUTH", os.Getenv("SUPER_SECURE_SECRETS"))
    return nil
}
```

## ws

get that ws goin like nothing:
```golang
package main

import(
    "context"
    "log/slog"
    "sync/errgroup"

    "golang.org/x/sync/errgroup"
    "github.com/rileyr/apic"
)

func main() {
    ws := apic.NewWSClient("ws://so-cool-ws-endpoint",
        apic.WithWSLoger(slog.Default()),
        apic.WithWSOnOpen(func(c *WSClient) error {
            slog.Default().Info("we are connected!")
            return nil
        }),
        apic.WithWSHandler(func(bts []byte) error {
            slog.Default().Info("we have something!", "message", string(bts))
            return nil
        }),
        apic.WithReconnectBackoff(time.Minute*5),
    }

    if err := ws.Start(context.Background()); err != nil {
        log.Printf("fatal err: %s\n", err.Error())
        os.Exit(1)
    }

    // or maybe the connection is feeding into a larger app,
    // the client can be run in an errgroup easily:
    wg, ctx := errgroup.WithContext(context.Background()) 
    wg.Go(func() error { return ws.Start(ctx) })
}
```
