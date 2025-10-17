# goRouter

A simple, lightweight REST supporting Router implemented in Go. 

## Features

- Static route handle handling (/home, /about) 
- Route handling with parameters (/users/{id}) 
- Method-based routing
- Chainable middleware pipeline
- Global and route specific middleware support
- Custom global panic recovery during execution
- Custom 404 handler
- Request logger middleware
- Radix tree based URL storage

### Planned features

- Router groups nesting (/api/v1/...)
- Route priority for faster lookup
- CORS middleware
- Authentication / Authorization middleware
- Rate limiting middleware
- Throttling middleware
- Compression middleware
- Websocket integration
- Context negotiation (JSON, XML, HTML automatic)
- pprof integration – probably with router groups.

## Creating a new instance

In order to initiate a new router entity, call the `New` function with all desired functional parameters.

```go
package main

import "github.com/balazskvancz/gorouter"

func main() {
  // A simple and functional router which will set all 
  // different features to default values. 
  // The port will be :8000, default 404 and HTTP OPTIONS method handler.
  // The by default response status will be 200,
  // and the maximum size of the a multipart/form-data will be 20Mb.
  r := gorouter.New()

  // Starts the listening. Be aware: this is a blocking method!
  r.Listen()
}
```

In case of using different settings, it is possible to decorate this factory method by passing various functions such as:

```go
var customNotFoundHandler = func (ctx gorouter.Context) {
  type res struct {
    Msg string `json:"msg"`
  }

  data := res{
    Msg: "not found",
  }

  ctx.SendJson(&data)
}

r := gorouter.New(
  // Sets up the router's address to :3000
  gorouter.WithAddress(3000),
  // Maximum of 50Mb formData body.
  gorouter.WithMaxBodySize(50<<20),
  // Sets the method in case of not finding a matching route.
  gorouter.WithNotFoundHandler(customNotFoundHandler),
)
```

### Listen

The basic mode to make the router start listening on the given port is by calling `Listen()`. It will be up and running until the context receives termination signal.

### ListenWithContext

The other way of starting the listening is by calling `ListenWithContext`, where you can pass a context as a parameter, which could have a cancellation or a timeout. Remember, is also ends the listening in case of an interrupt or a sigterm signal.

```go
r := gorouter.New()

ctx, cancel = context.WithCancel(context.Background())

go r.ListenWithContext(ctx)

// some logic...
cancel() // It stops the running of the router.
```

## Registering endpoints

You can register handlers with all the `HTTP methods` by calling `router.[Get|Post|Put...]` on the router instance, and providing an url and a function with the specified signature – called handler.

Of course, there is possibility to register routes with wildcard path parameters, which is essential in `REST` architectural style.

```go
r := gorouter.New()

r.Get("/api/products/{id}", func (ctx Context) {
  id := ctx.GetParam("id")
  ctx.WriteResponse([]byte(id))
})

r.Post("/api/products/{id}", func (ctx Context) {
  // doing some logic...
  ctx.Status(http.StatusOK) // Normal HTTP 200 response.
})
```

Every endpoint can have multiple – both global and local – middlewares registered, which execute before the handler. Keep in mind, if at least on middleware does not call the `.Next()` function, then the handler – and also the remaining middlewares – are not going to be executed.

```go
r := gorouter.New()

r.Get("/api/products/{id}", func (ctx Context) {
  id := ctx.GetParam("id")
	ctx.WriteResponse([]byte(id))
}).RegisterMiddlewares(func(ctx gorouter.Context) {
  fmt.Println(ctx.GetUrl())

  ctx.Next()
})

// The `RegisterMiddlewares` method takes a slice of MiddlewareFunc as parameter, 
// so you can register multiple middlewares at the same time.
// The order of executions will preserve the order of registration.

var (
  // Example of a MiddlewareFunc.
  mw1 = func (ctx gorouter.Context) {
    fmt.Println(ctx.GetUrl())
    
    ctx.Next()
  }

  mw2 = func (ctx gorouter.Context) {
    fmt.Println("Another one")

    ctx.Next()
  }

  r.Get("/api/welcome", func (ctx Context) {
	  ctx.WriteResponse([]byte("Welcome to this API!"))
  }).RegisterMiddlewares(mw2, mw1) // So the executes order will be the mw2, mw1 and the handler.
)

```

## Global middlewares

Beside the middleware functions that are attached to certain endpoints by registering it explicitly, there is a way to register middlewares on a global level. These middlewares are consists of two main parts: the first one is the prementioned `MiddlewareFunc`, and the second is the `matcher` – or multiple ones.

The `MiddlewareFunc` is responsible for doing the actual middleware logic and the `matcher` is for determining if a `Middleware` is opt for that certain Context.

Lets take an example here. You want a globally registered middleware which authorizes the request. You dont want every request to be examined, only if the route contains a the phrase `admin` in it.

You could achive this by creating your middleware like this:

```go
adminMWFn := func(ctx gorouter.Context) {
  if !strings.Contains(ctx.GetUrl(), "admin") {
    ctx.Next()

    return
  }
  if isAdmin := isAdmin(); isAdmin {
    ctx.Next()
    return
  }
  // Logging or any other activities in case of unsuccessful authorization.
}

adminMw := gorouter.NewMiddleware(adminMwFn)
// ...
```

However, this router takes a different approach. I believe, a `MiddlewareFunc` should only be responsible for the main logic and not for determining if the execution should take place or not.

Example: 

```go
adminMWFn := func(ctx gorouter.Context) {
  if isAdmin := isAdmin(); isAdmin {
    ctx.Next()

    return
  }
  // Logging or any other activities in case of unsuccessful authorization.
}

adminMwMatcher := func(ctx gorouter.Context) bool {
  return strings.Contains(ctx.GetUrl(), "admin") 
}

adminMw := gorouter.NewMiddleware(adminMwFn, adminMwMatcher)
// ...
```

This way you can separately test your `MiddlewareFunc` and also the `matcherFunc` if it is complex.

The `NewMiddleware` creates a new `Middleware` for global registration, and if you dont provide any matcher(s) then, the default one would be used, which means, it would match for each and every context.

Also, it is possible to register more than one matchers to a global Middleware. In this case, every matcher should be matching. This one is for code reusability.

The execution order follows this pattern:
  - execute all global – `preRunner` – middlewares that are matching for the Context,
  - looking up the registered handler from the tree based upon the method and the URL,
  - executing the attached middlewares to the endpoint – if there is any,
  - executing the handler,
  - executing all global – `postRunner` – middlewares that are matching for the Context.

### Pre and PostRunner global middlewares

The only difference between the these middlewares are the registration function.

```go
r := gorouter.New()

preMw := gorouter.NewMiddleware(func(ctx gorouter.Context) {
  // pre logic.
  ctx.Next()
})

postMw := gorouter.NewMiddleware(func(ctx gorouter.Context) {
  // post logic.
  ctx.Next()
})

r.RegisterMiddlewares(preMw)
r.RegisterPostMiddlewares(postMw)

```

## Context

The main way to interacting with the incoming request and the response is done by the abstraction that the `Context` implements. From reading the incoming body to writing the response, everything this done by this interface.
