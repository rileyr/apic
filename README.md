# apic

api clients, because who has time for that?

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
    client := apic.NewHTTPClient(
        "my-cool-yaml-api-root",
        apic.WithLogger(slog.Default()),
        apic.WithEncoder(yaml.Marshal),
        apic.WithDecoder(yaml.Unmarshal),
        apic.WithBefore(addAuthSignature),
    )

    type request struct{
        Foo string `yaml:"foo"`
        Bar struct {
            Biz int `yaml:"biz"`
        } `yaml:"bar"`
    }

    type response struct {
        Id   int    `yaml:"id"`
        Whiz string `yaml:"whiz"`
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
