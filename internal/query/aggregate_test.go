package query

import (
	"context"
	"testing"
)

func TestAggregateCount(t *testing.T) {
	engine, store := setupEngine(t)
	seedDocs(t, store, 30)

	res, err := engine.Aggregate(context.Background(), "posts", AggregateOptions{Op: AggregateOpCount})
	if err != nil {
		t.Fatal(err)
	}
	if res != 30 {
		t.Fatalf("count = %v, want 30", res)
	}
}

func TestAggregateSum(t *testing.T) {
	engine, store := setupEngine(t)
	seedDocs(t, store, 30)

	res, err := engine.Aggregate(context.Background(), "posts", AggregateOptions{Op: AggregateOpSum, Field: "views"})
	if err != nil {
		t.Fatal(err)
	}
	// views = i*10 for i in 0..29 → sum = 10 * (0+..+29) = 4350.
	if res != 4350 {
		t.Fatalf("sum = %v, want 4350", res)
	}
}

func TestAggregateSumWithFilter(t *testing.T) {
	engine, store := setupEngine(t)
	seedDocs(t, store, 30)

	res, err := engine.Aggregate(context.Background(), "posts", AggregateOptions{
		Op:     AggregateOpSum,
		Field:  "views",
		Filter: `status = "active"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	// status cycles draft/active/archived; active docs are i=1,4,...,28 with views=10..280.
	if res != 1450 {
		t.Fatalf("filtered sum = %v, want 1450", res)
	}
}

func TestAggregateSumUsesJsonNumbers(t *testing.T) {
	engine, store := setupEngine(t)
	seedDocs(t, store, 3)

	res, err := engine.Aggregate(context.Background(), "posts", AggregateOptions{Op: AggregateOpSum, Field: "rating"})
	if err != nil {
		t.Fatal(err)
	}
	var want float64
	for i := 0; i < 3; i++ {
		want += float64((i * 7) % 13)
	}
	if res != want {
		t.Fatalf("sum rating = %v, want %v", res, want)
	}
}

func TestAggregateSumRequiresField(t *testing.T) {
	engine, store := setupEngine(t)
	seedDocs(t, store, 3)
	if _, err := engine.Aggregate(context.Background(), "posts", AggregateOptions{Op: AggregateOpSum}); err == nil {
		t.Fatal("expected error for sum without field")
	}
}

func TestAggregateInvalidOp(t *testing.T) {
	engine, store := setupEngine(t)
	seedDocs(t, store, 3)
	if _, err := engine.Aggregate(context.Background(), "posts", AggregateOptions{Op: "avg"}); err == nil {
		t.Fatal("expected error for unsupported op")
	}
}
