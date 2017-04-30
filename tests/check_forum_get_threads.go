package tests

import (
	"github.com/bozaro/tech-db-forum/generated/client"
	"github.com/bozaro/tech-db-forum/generated/client/operations"
	"github.com/bozaro/tech-db-forum/generated/models"
	"github.com/go-openapi/strfmt"
	"math/rand"
	"sort"
	"time"
)

func init() {
	Register(Checker{
		Name:        "forum_get_threads_simple",
		Description: "",
		FnCheck:     Modifications(CheckForumGetThreadsSimple),
		Deps: []string{
			"thread_create_simple",
		},
	})
	Register(Checker{
		Name:        "forum_get_threads_notfound",
		Description: "",
		FnCheck:     Modifications(CheckForumGetThreadsNotFound),
		Deps: []string{
			"thread_create_simple",
		},
	})
	PerfRegister(PerfTest{
		Name:   "forum_get_threads_success",
		Mode:   ModeRead,
		Weight: WeightNormal,
		FnPerf: PerfForumGetThreadsSuccess,
	})
	PerfRegister(PerfTest{
		Name:   "forum_get_threads_not_found",
		Mode:   ModeRead,
		Weight: WeightRare,
		FnPerf: PerfForumGetThreadsNotFound,
	})
}

type PThreadByCreated []*PThread

func (a PThreadByCreated) Len() int      { return len(a) }
func (a PThreadByCreated) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a PThreadByCreated) Less(i, j int) bool {
	time_i := time.Time(a[i].Created)
	time_j := time.Time(a[j].Created)
	return time_i.Before(time_j)
}

func filterThread(data interface{}) interface{} {
	thread := data.(*models.Thread)
	if thread.Created != nil {
		created := strfmt.DateTime(time.Time(*thread.Created).UTC())
		thread.Created = &created
	}
	return thread
}

func filterThreads(data interface{}) interface{} {
	threads := data.(*[]models.Thread)
	for i := range *threads {
		filterThread(&(*threads)[i])
	}
	return threads
}

func CheckForumGetThreadsSimple(c *client.Forum, m *Modify) {
	forum := CreateForum(c, nil, nil)
	threads := []models.Thread{}
	created := time.Now()
	created.Round(time.Millisecond)
	for i := 0; i < 10; i++ {
		thread := CreateThread(c, nil, forum, nil)
		threads = append(threads, *thread)
	}
	sort.Sort(ThreadByCreated(threads))

	var desc *bool

	// Desc
	small := time.Millisecond
	switch m.Int(3) {
	case 1:
		v := bool(true)
		small = -small
		desc = &v
		sort.Sort(sort.Reverse(ThreadByCreated(threads)))
	case 2:
		v := bool(false)
		desc = &v
	}

	// Slug
	slug := m.Case(forum.Slug)

	// Check read all
	c.Operations.ForumGetThreads(operations.NewForumGetThreadsParams().
		WithSlug(slug).
		WithDesc(desc).
		WithContext(Expected(200, &threads, filterThreads)))

	// Check read by 4 records
	limit := int32(4)
	var since *strfmt.DateTime = nil
	for n := 0; n < len(threads); n += int(limit) - 1 {
		m := n + int(limit)
		if m > len(threads) {
			m = len(threads)
		}
		expected := threads[n:m]
		c.Operations.ForumGetThreads(operations.NewForumGetThreadsParams().
			WithSlug(slug).
			WithLimit(&limit).
			WithDesc(desc).
			WithSince(since).
			WithContext(Expected(200, &expected, filterThreads)))
		since = threads[m-1].Created
	}

	// Check read after all
	after_last := strfmt.DateTime(time.Time(*threads[len(threads)-1].Created).Add(small))
	c.Operations.ForumGetThreads(operations.NewForumGetThreadsParams().
		WithSlug(slug).
		WithLimit(&limit).
		WithDesc(desc).
		WithSince(&after_last).
		WithContext(Expected(200, &[]models.Thread{}, nil)))
}

func CheckForumGetThreadsNotFound(c *client.Forum, m *Modify) {
	var limit *int32
	var since *strfmt.DateTime
	var desc *bool

	forum := RandomForum()
	// Limit
	if m.Bool() {
		v := int32(10)
		limit = &v
	}
	// Since
	if m.Bool() {
		v := strfmt.DateTime(time.Now())
		since = &v
	}
	// Desc
	switch m.Int(3) {
	case 1:
		v := bool(true)
		desc = &v
	case 2:
		v := bool(false)
		desc = &v
	}

	// Check
	_, err := c.Operations.ForumGetThreads(operations.NewForumGetThreadsParams().
		WithSlug(forum.Slug).
		WithLimit(limit).
		WithSince(since).
		WithDesc(desc).
		WithContext(Expected(404, nil, nil)))
	CheckIsType(operations.NewForumGetThreadsNotFound(), err)
}

func PerfForumGetThreadsSuccess(p *Perf) {
	forum := p.data.GetForum(-1)
	version := forum.Version

	slug := forum.Slug
	limit := GetRandomLimit()
	var since *strfmt.DateTime
	if rand.Int()&1 == 0 {
		since = &p.data.GetThread(-1).Created
	}
	desc := GetRandomDesc()
	result, err := p.c.Operations.ForumGetThreads(operations.NewForumGetThreadsParams().
		WithSlug(GetRandomCase(slug)).
		WithLimit(&limit).
		WithSince(since).
		WithDesc(desc).
		WithContext(Expected(404, nil, nil)))

	CheckNil(err)

	p.Validate(func(v PerfValidator) {
		if v.CheckVersion(version, forum.Version) {
			expected := p.data.GetForumThreads(forum)
			asc := (desc == nil) || (*desc == false)
			// Filter
			if since != nil {
				threads := expected
				expected = []*PThread{}
				t1 := time.Time(*since)
				for _, thread := range threads {
					t2 := time.Time(thread.Created)
					if (asc == t2.After(t1)) || t1.Equal(t2) {
						expected = append(expected, thread)
					}
				}
			}
			// Sort
			var sorter sort.Interface = PThreadByCreated(expected)
			if !asc {
				sorter = sort.Reverse(sorter)
			}
			sort.Sort(sorter)
			// Check
			if len(expected) > int(limit) {
				expected = expected[0:limit]
			}

			payload := result.Payload
			v.CheckInt(len(expected), len(payload), "len()")
			for i, item := range expected {
				item.Validate(v, payload[i], item.Version)
			}
			v.Finish(version, forum.Version)
		}
	})
}

func PerfForumGetThreadsNotFound(p *Perf) {
	slug := RandomForum().Slug
	limit := GetRandomLimit()
	var since *strfmt.DateTime
	if rand.Int()&1 == 0 {
		since = &p.data.GetThread(-1).Created
	}
	desc := GetRandomDesc()
	_, err := p.c.Operations.ForumGetThreads(operations.NewForumGetThreadsParams().
		WithSlug(slug).
		WithLimit(&limit).
		WithSince(since).
		WithDesc(desc).
		WithContext(Expected(404, nil, nil)))
	CheckIsType(operations.NewForumGetThreadsNotFound(), err)
}
