## 1. Reproduce the bug

- [ ] 1.1 Add `TestRetryNReportsPriorErrorWhenLaterAttemptReturnsNil` to
      `main_test.go`. Call `retryN(3, func() (bool, error)` — first call
      returns `(false, priorErr)`, subsequent calls return `(false, nil)`.
      Assert the result equals `priorErr`, not `nil`.
- [ ] 1.2 Run `go test ./...`. Confirm the test fails (`retryN` returns
      `nil` because the loop clobbers `priorErr`).

## 2. Refactor signatures

- [ ] 2.1 Change `retryN` signature to `func(int, func() error) error`.
      Body becomes:
      ```go
      var err error
      for range n {
          if err = f(); err == nil {
              return nil
          }
      }
      return err
      ```
- [ ] 2.2 Change `Watcher.work` to return `error` instead of `(bool, error)`.
      Keep the internal `done` local — it still gates the post-agent
      `git add` / `git commit`. Replace `return false, err` with
      `return err`, `return true, nil` with `return nil`, and
      `return done, nil` with `return nil`.
- [ ] 2.3 Update the caller in `Watcher.runOnce` to match:
      ```go
      if err := retryN(w.RetyCount, func() error {
          return w.work(ctx, repo)
      }); err != nil { ... }
      ```
- [ ] 2.4 Delete `TestRetryNReportsPriorErrorWhenLaterAttemptReturnsNil`
      (it no longer compiles under the new signature). Its rationale moves
      to a code comment on `retryN`:
      ```go
      // ponytail: previously took (bool, error); the (false, nil) state
      // could clobber a prior error and report silent success. Contract
      // collapsed to func() error so the bug class is unreachable.
      ```

## 3. Update existing tests for new signature

- [ ] 3.1 In `TestWorkCommitsOnSuccess`, change
      `done, err := w.work(ctx, repo)` to `err := w.work(ctx, repo)`.
      Remove the `if !done { t.Fatal("expected done=true") }` block —
      the new contract's pass/fail signal is `err`.

## 4. Add new-contract property test

- [ ] 4.1 Add `TestRetryNReturnsLastErrorWhenAllAttemptsFail` to
      `main_test.go`. Call `retryN(3, func() error)` returning
      `errors.New("a")`, `errors.New("b")`, `errors.New("c")`.
      Assert the result is `errors.New("c")`.
- [ ] 4.2 Run `go test ./...`. Confirm green.

## 5. Verify

- [ ] 5.1 `go vet ./...` clean.
- [ ] 5.2 `go build ./...` clean.
- [ ] 5.3 `go test -race ./...` green.
- [ ] 5.4 Manual read-through of `main.go`: confirm no stray `bool` returns
      from `work`, no `(bool, error)` lambdas in `runOnce`, no dead
      imports (`slices` is now unused if not referenced elsewhere — check).