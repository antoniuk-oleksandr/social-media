package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"
)

type fakeClient struct {
	n      int
	err    error
	inputs []*dynamodb.CreateTableInput
}

func (f *fakeClient) CreateTable(_ context.Context, input *dynamodb.CreateTableInput, _ ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error) {
	f.n++
	f.inputs = append(f.inputs, input)
	return &dynamodb.CreateTableOutput{}, f.err
}

var _ = Describe("RunCreatesTablesAndAllowsAlreadyExists", func() {
	It("runs the test", func() {
		os.Setenv("DYNAMODB_ENDPOINT", "http://local")
		defer os.Unsetenv("DYNAMODB_ENDPOINT")
		f := &fakeClient{err: &types.ResourceInUseException{}}
		count := 0
		e := run(context.Background(), func(context.Context, string) (aws.Config, error) { return aws.Config{}, nil }, func(aws.Config) tableCreator { return f }, func(string, ...any) (int, error) { count++; return 0, nil })
		if e != nil || f.n != 3 || count != 3 {
			Expect(false).To(BeTrue(), fmt.Sprintf("e=%v n=%d count=%d", e, f.n, count))
		}
		if len(f.inputs) != 3 {
			Expect(false).To(BeTrue(), fmt.Sprintf("captured inputs=%d, want 3", len(f.inputs)))
		}
		assertIndex := func(table int, name, attribute string) {
			input := f.inputs[table]
			for _, index := range input.GlobalSecondaryIndexes {
				if aws.ToString(index.IndexName) == name {
					if len(index.KeySchema) != 1 || aws.ToString(index.KeySchema[0].AttributeName) != attribute || index.KeySchema[0].KeyType != types.KeyTypeHash {
						Expect(false).To(BeTrue(), fmt.Sprintf("index %q has schema %#v", name, index.KeySchema))
					}
					if index.Projection == nil || index.Projection.ProjectionType != types.ProjectionTypeAll {
						Expect(false).To(BeTrue(), fmt.Sprintf("index %q projection=%#v, want ALL", name, index.Projection))
					}
					return
				}
			}
			Expect(false).To(BeTrue(), fmt.Sprintf("index %q not found in %#v", name, input.GlobalSecondaryIndexes))
		}
		assertIndex(0, "EmailIndex", "email")
		assertIndex(0, "UsernameIndex", "username")
		assertIndex(2, "SessionIdIndex", "sessionId")
		if len(f.inputs[1].GlobalSecondaryIndexes) != 0 {
			Expect(false).To(BeTrue(), fmt.Sprintf("uniques GSIs=%d, want 0", len(f.inputs[1].GlobalSecondaryIndexes)))
		}

	})
})
var _ = Describe("RunPropagatesLoadAndTableErrors", func() {
	It("runs the test", func() {
		want := errors.New("load")
		if e := run(context.Background(), func(context.Context, string) (aws.Config, error) { return aws.Config{}, want }, nil, nil); !errors.Is(e, want) {
			Expect(false).To(BeTrue(), fmt.Sprint(e))
		}
		f := &fakeClient{err: want}
		e := run(context.Background(), func(context.Context, string) (aws.Config, error) { return aws.Config{}, nil }, func(aws.Config) tableCreator { return f }, func(string, ...any) (int, error) { return 0, nil })
		if !errors.Is(e, want) {
			Expect(false).To(BeTrue(), fmt.Sprint(e))
		}

	})
})
var _ = Describe("EnvAndErrorAs", func() {
	It("runs the test", func() {
		const k = "INIT_TEST_ENV"
		os.Unsetenv(k)
		if env(k, "fallback") != "fallback" {
			Expect(false).To(BeTrue())
		}
		os.Setenv(k, "value")
		defer os.Unsetenv(k)
		if env(k, "fallback") != "value" {
			Expect(false).To(BeTrue())
		}
		var target *types.ResourceInUseException
		if !errorAs(&types.ResourceInUseException{}, &target) || errorAs(errors.New("no"), &target) {
			Expect(false).To(BeTrue())
		}
		_ = aws.Config{}

	})
})

var _ = Describe("MainPanicsWhenLocalDynamoDBIsUnavailable", func() {
	It("runs the test", func() {
		os.Setenv("AWS_REGION", "us-east-1")
		DeferCleanup(func() { _ = os.Unsetenv("AWS_REGION") })
		os.Setenv("DYNAMODB_ENDPOINT", "http://127.0.0.1:1")
		DeferCleanup(func() { _ = os.Unsetenv("DYNAMODB_ENDPOINT") })

		defer func() {
			if recovered := recover(); recovered == nil {
				Expect(false).To(BeTrue(), fmt.Sprint("main should panic when initialization cannot reach DynamoDB"))
			} else {
				GinkgoWriter.Write([]byte(fmt.Sprintf("main panic: %v", recovered)))
			}
		}()
		main()

	})
})
