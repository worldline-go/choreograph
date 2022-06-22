# Choreograph

Package choreograph contains sequentially executing processor.
Such a way that subsequent steps are executed only when the preceding step has succeeded (job finished successfully).
Each step also has a function that checks if the step should be executed, if so, the work
specified in the step is executed, otherwise the step is skipped and the next step in the queue is passed.

## Install
Add `github.com/worldline-go/choreograph` to your `go.mod`

or simply

```sh
go get github.com/worldline-go/choreograph
```

## Usage
For each step created, a name, pre-check and job functions are required.
Callbacks (pre-check, job) can take different forms:

- `func(ctx context.Context) error`

    Simplest form of callback, receives a context (with the data you put there like DB connection etc.) and
    error on output which indicates successful execution of it.

- `func(ctx context.Context) (output string, err error)`

    Callback function can also return an additional output of any type, it can even be a structure!

- `func(ctx context.Context, input int) error`

    If you define a second input parameter (it can also take any type), data you passed on Run() method can be found here.
    This is really helpful if you want to run same steps for multiple records.

- `func(ctx context.Context, input string) (output float64, err error)`

    Also, it's possible to mix both and have custom input and output!

Keep in mind that for callback requires context.Context to be the first parameter and error need to be last returned.

## Additional notes

- use `NewCoordinator` method to create an instance!
- error returned from pre-check function stops step job from being executed
- use `ErrExecutionCanceled` if you need a cancellation in pre-check job
(DB connection issue, network issue which prevents pre-check to run)
- error returned from job function stops further execution of any step
- a `DataBag` object is added to coordinator which keeps returned data from all callbacks,
you can retrieve if using `GetDataBagFromCtx` method and then get the data from pre-check's and job's by the step name
- each execution of the `Run` method clears the `DataBag`
- only 'fatal' errors which stops process execution will be returned from `Run` method, those are:
  - errors returned from any job
  - errors returned by pre-check if it's an `ErrExecutionCanceled`
- all errors are stored in the stack and can be retrieved with `GetExecutionErrors` method (pre-check and job errors)
- each execution of the `Run` method clears the error stack

## Examples

### Simple

```go
package main

import (
	"context"

	"github.com/worldline-go/choreograph"
)

func main() {
	ctx := context.Background()

	coordinator, err := choreograph.NewCoordinator(ctx)
	if err != nil {
		panic(err)
	}

	err = coordinator.AddStep(&choreograph.Step{
		Name: "first",
		PreCheck: func(ctx context.Context) error {
			// do check before job execution
			return nil
		},
		Job: func(ctx context.Context) error {
			// execute your job
			return nil
		},
	})

	if err != nil {
		panic(err)
	}

	if err := coordinator.Run(nil); err != nil {
		panic(err)
	}
}

```

### Using context values and callback returns from data bag

```go
package main

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"
	"github.com/worldline-go/choreograph"
)

const CtxDbKey = "ctx_db_conn"

type statementLine struct {
	statementId string
	amount      float64
}

func main() {
	inputs := []statementLine{
		{statementId: "a", amount: 3.13},
		{statementId: "b", amount: 6.11},
		{statementId: "c", amount: 76.33},
	}

	ctx := context.Background()

	conn, err := sql.Open("postgres", "WHATEVER CONNECTION STRING")
	if err != nil {
		panic(err)
	}

	ctx = context.WithValue(context.Background(), CtxDbKey, conn)

	coordinator, err := choreograph.NewCoordinator(ctx)
	if err != nil {
		panic(err)
	}

	steps := getSteps()

	for _, s := range steps {
		err = coordinator.AddStep(s)
		if err != nil {
			panic(err)
		}
	}

	for _, i := range inputs {
		if err := coordinator.Run(i); err != nil {
			panic(err)
		}
	}
}

func getSteps() []*choreograph.Step {
	return []*choreograph.Step{
		{
			Name: "create statement",
			PreCheck: func(ctx context.Context, input statementLine) (string, error) {
				// getting db connection from context! it's there but always check when casting
				conn, ok := ctx.Value(CtxDbKey).(*sql.DB)
				if !ok {
					// ErrExecutionCanceled cancels further execution of callback after because this would be fatal here
					return "", errors.Wrap(choreograph.ErrExecutionCanceled, "unexpected behaviour in create statement pre-check!")
				}

				if checkIfStatementExists(conn, input.statementId) {
					// returning error from pre-check prevents from executing a job in this step
					// this does not stop execution of other steps (except if special ErrExecutionCanceled is returned)
					// additionally we want to return record ID of existing statement
					return getStatementID(conn, input.statementId), errors.New("statement exists!")
				}

				// no error on check, statement doesn't exist, go to job
				return "", nil
			},
			Job: func(ctx context.Context, input statementLine) (string, error) {
				// getting db connection from context
				conn, ok := ctx.Value(CtxDbKey).(*sql.DB)
				if !ok {
					// any error from job will stop execution of further steps and callbacks, even if no ErrExecutionCanceled
					// is returned
					return "", errors.New("unexpected behaviour in create statement job!")
				}

				recordID, err := createStatement(conn, input.statementId)
				if err != nil {
					return "", errors.New("oh no, failed with creating a statement")
				}

				return recordID, nil
			},
		},
		{
			Name: "create statement lines",
			PreCheck: func(ctx context.Context, input statementLine) error {
				conn, ok := ctx.Value(CtxDbKey).(*sql.DB)
				if !ok {
					return errors.Wrap(choreograph.ErrExecutionCanceled, "unexpected behaviour in create statement line pre-check!")
				}

				if checkIfStatementLineExists(conn, input.statementId, input.amount) {
					return errors.New("statement line exists!")
				}

				// no error on check, statement line doesn't exist, go to job
				return nil
			},
			Job: func(ctx context.Context, input statementLine) error {
				conn, ok := ctx.Value(CtxDbKey).(*sql.DB)
				if !ok {
					return errors.New("unexpected behaviour in create statement line job!")
				}

				dataBag, err := choreograph.GetDataBagFromCtx(ctx)
				if err != nil {
					return errors.Wrap(err, "this context doesn't have a data bag!")
				}

				statementRecordId, exists := dataBag.GetJobData("create statement")
				if !exists {
					return errors.New("statement record ID not found!")
				}

				statementRecordIdStr, ok := statementRecordId.(string)
				if !ok {
					return errors.New("expected statement record ID to be string!")
				}

				if statementRecordIdStr == "" {
					// if previous job returned empty record ID it means that it was already in place, we need to take
					// one which pre-check returned
					statementRecordId, exists = dataBag.GetPreCheckData("create statement")
					if !exists {
						return errors.New("statement record ID not found in pre-check as well!")
					}

					statementRecordIdStr, ok = statementRecordId.(string)
					if !ok {
						return errors.New("expected statement record ID to be string!")
					}
				}

				if createStatementLines(conn, statementRecordIdStr, input) != nil {
					return errors.New("oh no, failed with creating a statement line")
				}

				return nil
			},
		},
	}
}

func createStatementLines(conn *sql.DB, id string, input statementLine) error {
	// insert statement line with proper record ID of the statement
	// ...
	return nil
}

func checkIfStatementExists(conn *sql.DB, statementID string) bool {
	// check if statement is already in DB with some SQL
	// ...
	return true
}

func getStatementID(conn *sql.DB, statementID string) string {
	// get statement record ID
	// ...
	return "some record ID"
}

func createStatement(conn *sql.DB, id string) (string, error) {
	// check if statement is already in DB with some SQL
	// ...
	return "some record ID", nil
}

func checkIfStatementLineExists(conn *sql.DB, statementID string, amount float64) bool {
	// check if statement is already in DB with some SQL
	// ...
	return true
}

```