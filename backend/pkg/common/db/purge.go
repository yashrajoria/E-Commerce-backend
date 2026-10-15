package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
)

// PurgeJob deletes rows of Table matching Where. Both are code constants,
// never user input.
type PurgeJob struct{ Table, Where string }

const purgeBatch = 5000

// StartPurger runs jobs periodically until ctx is done.
// Deletes are batched so a run over a big backlog doesn't hold a long lock.
// An initial delay is applied to prevent startup connection spikes when multiple services boot.
func StartPurger(ctx context.Context, gdb *gorm.DB, every time.Duration, jobs ...PurgeJob) {
	go func() {
		initialDelay := time.Minute
		if every < initialDelay {
			initialDelay = every
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(initialDelay):
		}

		t := time.NewTicker(every)
		defer t.Stop()
		for {
			for _, j := range jobs {
				purge(ctx, gdb, j)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func purge(ctx context.Context, gdb *gorm.DB, j PurgeJob) {
	q := fmt.Sprintf("DELETE FROM %[1]s WHERE ctid IN (SELECT ctid FROM %[1]s WHERE %[2]s LIMIT %[3]d)", j.Table, j.Where, purgeBatch)
	var total int64
	for ctx.Err() == nil {
		res := gdb.WithContext(ctx).Exec(q)
		if res.Error != nil {
			log.Printf("purge %s: %v", j.Table, res.Error)
			return
		}
		total += res.RowsAffected
		if res.RowsAffected < purgeBatch {
			break
		}
	}
	if total > 0 {
		log.Printf("purge %s: deleted %d rows", j.Table, total)
	}
}
