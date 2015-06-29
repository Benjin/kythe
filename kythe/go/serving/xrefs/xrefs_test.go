/*
 * Copyright 2015 Google Inc. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package xrefs

import (
	"bytes"
	"log"
	"reflect"
	"sort"
	"strings"
	"testing"

	"kythe.io/kythe/go/storage/table"
	"kythe.io/kythe/go/test/testutil"
	"kythe.io/kythe/go/util/stringset"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	srvpb "kythe.io/kythe/proto/serving_proto"
	xpb "kythe.io/kythe/proto/xref_proto"
)

var (
	ctx = context.Background()
	tbl = &testTable{
		Nodes: []*srvpb.Node{
			{
				Ticket: "kythe://someCorpus?lang=otpl#signature",
				Fact:   makeFactList("/kythe/node/kind", "testNode"),
			}, {
				Ticket: "kythe://someCorpus#aTicketSig",
				Fact:   makeFactList("/kythe/node/kind", "testNode"),
			}, {
				Ticket: "kythe://someCorpus?lang=otpl#something",
				Fact: makeFactList(
					"/kythe/node/kind", "name",
					"/some/other/fact", "value",
				),
			}, {
				Ticket: "kythe://someCorpus?lang=otpl#sig2",
				Fact:   makeFactList("/kythe/node/kind", "name"),
			}, {
				Ticket: "kythe://someCorpus?lang=otpl#sig3",
				Fact:   makeFactList("/kythe/node/kind", "name"),
			}, {
				Ticket: "kythe://someCorpus?lang=otpl#sig4",
				Fact:   makeFactList("/kythe/node/kind", "name"),
			}, {
				Ticket: "kythe://someCorpus?lang=otpl?path=/some/valid/path#a83md71",
				Fact: makeFactList(
					"/kythe/node/kind", "file",
					"/kythe/text", "; some file content here\nfinal line\n",
					"/kythe/text/encoding", "utf-8",
				),
			},
		},
		EdgeSets: []*srvpb.PagedEdgeSet{
			{
				TotalEdges: 3,
				EdgeSet: &srvpb.EdgeSet{
					SourceTicket: "kythe://someCorpus?lang=otpl#something",
					Group: []*srvpb.EdgeSet_Group{
						{
							Kind: "someEdgeKind",
							TargetTicket: []string{
								"kythe://someCorpus#aTicketSig",
							},
						},
						{
							Kind: "anotherEdge",
							TargetTicket: []string{
								"kythe://someCorpus#aTicketSig",
								"kythe://someCorpus?lang=otpl#signature",
							},
						},
					},
				},
			}, {
				TotalEdges: 3,
				EdgeSet: &srvpb.EdgeSet{
					SourceTicket: "kythe://someCorpus?lang=otpl#signature",
				},
				PageIndex: []*srvpb.PageIndex{{
					PageKey:   "firstPage",
					EdgeKind:  "someEdgeKind",
					EdgeCount: 2,
				}, {
					PageKey:   "secondPage",
					EdgeKind:  "anotherEdge",
					EdgeCount: 1,
				}},
			},
		},
		EdgePages: []*srvpb.EdgePage{
			{
				PageKey:      "orphanedEdgePage",
				SourceTicket: "kythe://someCorpus?lang=otpl#something",
			}, {
				PageKey:      "firstPage",
				SourceTicket: "kythe://someCorpus?lang=otpl#signature",
				EdgesGroup: &srvpb.EdgeSet_Group{
					Kind: "someEdgeKind",
					TargetTicket: []string{
						"kythe://someCorpus?lang=otpl#sig3",
						"kythe://someCorpus?lang=otpl#sig4",
					},
				},
			}, {
				PageKey:      "secondPage",
				SourceTicket: "kythe://someCorpus?lang=otpl#signature",
				EdgesGroup: &srvpb.EdgeSet_Group{
					Kind: "anotherEdge",
					TargetTicket: []string{
						"kythe://someCorpus?lang=otpl#sig2",
					},
				},
			},
		},
		Decorations: []*srvpb.FileDecorations{
			{
				FileTicket: "kythe://someCorpus?lang=otpl?path=/some/valid/path#a83md71",
				SourceText: []byte("; some file content here\nfinal line\n"),
				Encoding:   "utf-8",
			},
			{
				FileTicket: "kythe://someCorpus?lang=otpl?path=/a/path#b7te37tn4",
				SourceText: []byte("(defn map [f coll]\n  (if (empty? coll)\n    []\n    (cons (f (first coll)) (map f (rest coll)))))\n"),
				Encoding:   "utf-8",
				Decoration: []*srvpb.FileDecorations_Decoration{
					{
						Anchor: &srvpb.FileDecorations_Decoration_Anchor{
							Ticket:      "kythe://c?lang=otpl?path=/a/path#6-9",
							StartOffset: 6,
							EndOffset:   9,
						},
						Kind:         "/kythe/defines",
						TargetTicket: "kythe://c?lang=otpl?path=/a/path#map",
					},
				},
			},
		},
	}
)

func TestNodes(t *testing.T) {
	st := tbl.Construct(t)

	for _, node := range tbl.Nodes {
		reply, err := st.Nodes(ctx, &xpb.NodesRequest{
			Ticket: []string{node.Ticket},
		})
		testutil.FatalOnErrT(t, "NodesRequest error: %v", err)

		if len(reply.Node) != 1 {
			t.Fatalf("Expected 1 node in reply; found %d: {%v}", len(reply.Node), reply)
		} else if expected := nodeInfo(node); !reflect.DeepEqual(reply.Node[0], expected) {
			t.Fatalf("Expected {%v}; received {%v}", expected, reply.Node[0])
		}
	}

	var tickets []string
	var expected []*xpb.NodeInfo
	for _, n := range tbl.Nodes {
		tickets = append(tickets, n.Ticket)
		expected = append(expected, nodeInfo(n))
	}
	reply, err := st.Nodes(ctx, &xpb.NodesRequest{Ticket: tickets})
	testutil.FatalOnErrT(t, "NodesRequest error: %v", err)

	sort.Sort(byNodeTicket(expected))
	sort.Sort(byNodeTicket(reply.Node))

	if !reflect.DeepEqual(expected, reply.Node) {
		t.Fatalf("Expected {%v}; received {%v}", expected, reply.Node)
	}
}

func TestNodesMissing(t *testing.T) {
	st := tbl.Construct(t)
	reply, err := st.Nodes(ctx, &xpb.NodesRequest{
		Ticket: []string{"someMissingTicket"},
	})
	testutil.FatalOnErrT(t, "NodesRequest error: %v", err)

	if len(reply.Node) > 0 {
		t.Fatalf("Received unexpected reply for missing node: {%v}", reply)
	}
}

func TestEdgesSinglePage(t *testing.T) {
	tests := []struct {
		Tickets []string
		Kinds   []string

		EdgeSet *srvpb.PagedEdgeSet
	}{{
		Tickets: []string{tbl.EdgeSets[0].EdgeSet.SourceTicket},
		EdgeSet: tbl.EdgeSets[0],
	}, {
		Tickets: []string{tbl.EdgeSets[0].EdgeSet.SourceTicket},
		Kinds:   []string{"someEdgeKind", "anotherEdge"},
		EdgeSet: tbl.EdgeSets[0],
	}}

	st := tbl.Construct(t)

	for _, test := range tests {
		reply, err := st.Edges(ctx, &xpb.EdgesRequest{
			Ticket: test.Tickets,
			Kind:   test.Kinds,
		})
		testutil.FatalOnErrT(t, "EdgesRequest error: %v", err)

		if len(reply.Node) > 0 {
			t.Errorf("Received unexpected nodes in EdgesReply: {%v}", reply)
		}
		if reply.NextPageToken != "" {
			t.Errorf("Received unexpected next_page_token in EdgesReply: {%v}", reply)
		}
		if len(reply.EdgeSet) != 1 {
			t.Errorf("Expected 1 EdgeSet in EdgesReply; found %d: {%v}", len(reply.EdgeSet), reply)
		}

		expected := edgeSet(test.Kinds, test.EdgeSet, nil)
		if !reflect.DeepEqual(reply.EdgeSet[0], expected) {
			t.Errorf("Expected {%v}; found {%v}", expected, reply.EdgeSet)
		}
	}
}

func TestEdgesMultiPage(t *testing.T) {
	tests := []struct {
		Tickets []string
		Kinds   []string

		EdgeSet *srvpb.PagedEdgeSet
		Pages   []*srvpb.EdgePage
	}{{
		Tickets: []string{tbl.EdgeSets[1].EdgeSet.SourceTicket},
		EdgeSet: tbl.EdgeSets[1],
		Pages:   []*srvpb.EdgePage{tbl.EdgePages[1], tbl.EdgePages[2]},
	}}

	st := tbl.Construct(t)

	for _, test := range tests {
		reply, err := st.Edges(ctx, &xpb.EdgesRequest{
			Ticket: test.Tickets,
			Kind:   test.Kinds,
		})
		testutil.FatalOnErrT(t, "EdgesRequest error: %v", err)

		if len(reply.Node) > 0 {
			t.Errorf("Received unexpected nodes in EdgesReply: {%v}", reply)
		}
		if reply.NextPageToken != "" {
			t.Errorf("Received unexpected next_page_token in EdgesReply: {%v}", reply)
		}
		if len(reply.EdgeSet) != 1 {
			t.Errorf("Expected 1 EdgeSet in EdgesReply; found %d: {%v}", len(reply.EdgeSet), reply)
		}

		expected := edgeSet(test.Kinds, test.EdgeSet, test.Pages)
		if !reflect.DeepEqual(reply.EdgeSet[0], expected) {
			t.Errorf("Expected {%v}; found {%v}", expected, reply.EdgeSet)
		}
	}
}

func TestEdgesMissing(t *testing.T) {
	st := tbl.Construct(t)
	reply, err := st.Edges(ctx, &xpb.EdgesRequest{
		Ticket: []string{"someMissingTicket"},
	})
	testutil.FatalOnErrT(t, "EdgesRequest error: %v", err)

	if len(reply.EdgeSet) > 0 || len(reply.Node) > 0 || reply.NextPageToken != "" {
		t.Fatalf("Received unexpected reply for missing edges: {%v}", reply)
	}
}

func TestDecorationsRefs(t *testing.T) {
	d := tbl.Decorations[1]

	st := tbl.Construct(t)
	reply, err := st.Decorations(ctx, &xpb.DecorationsRequest{
		Location:   &xpb.Location{Ticket: d.FileTicket},
		References: true,
	})
	testutil.FatalOnErrT(t, "DecorationsRequest error: %v", err)

	if len(reply.SourceText) != 0 {
		t.Errorf("Unexpected source text: %q", string(d.SourceText))
	}
	if reply.Encoding != "" {
		t.Errorf("Unexpected encoding: %q", d.Encoding)
	}

	expected := refs(d.Decoration)
	if !reflect.DeepEqual(expected, reply.Reference) {
		t.Fatalf("Expected references %v; found %v", expected, reply.Reference)
	}
}

func TestDecorationsNotFound(t *testing.T) {
	st := tbl.Construct(t)
	reply, err := st.Decorations(ctx, &xpb.DecorationsRequest{
		Location: &xpb.Location{
			Ticket: "someMissingFileTicket",
		},
	})

	if err == nil {
		t.Fatalf("Unexpected DecorationsReply: {%v}", reply)
	} else if !strings.Contains(err.Error(), "decorations not found for file") {
		t.Fatalf("Unexpected Decorations error: %v", err)
	}
}

func TestDecorationsEmpty(t *testing.T) {
	st := tbl.Construct(t)
	reply, err := st.Decorations(ctx, &xpb.DecorationsRequest{
		Location: &xpb.Location{
			Ticket: tbl.Decorations[0].FileTicket,
		},
		References: true,
	})
	testutil.FatalOnErrT(t, "DecorationsRequest error: %v", err)

	if len(reply.Reference) > 0 {
		t.Fatalf("Unexpected DecorationsReply: {%v}", reply)
	}
}

func TestDecorationsSourceText(t *testing.T) {
	expected := tbl.Decorations[0]

	st := tbl.Construct(t)
	reply, err := st.Decorations(ctx, &xpb.DecorationsRequest{
		Location:   &xpb.Location{Ticket: expected.FileTicket},
		SourceText: true,
	})
	testutil.FatalOnErrT(t, "DecorationsRequest error: %v", err)

	if !bytes.Equal(reply.SourceText, expected.SourceText) {
		t.Errorf("Expected source text %q; found %q", string(expected.SourceText), string(reply.SourceText))
	}
	if reply.Encoding != expected.Encoding {
		t.Errorf("Expected source text %q; found %q", expected.Encoding, reply.Encoding)
	}
	if len(reply.Reference) > 0 {
		t.Errorf("Unexpected references in DecorationsReply %v", reply.Reference)
	}
}

type byNodeTicket []*xpb.NodeInfo

// Implement the sort.Interface
func (s byNodeTicket) Len() int           { return len(s) }
func (s byNodeTicket) Less(i, j int) bool { return s[i].Ticket < s[j].Ticket }
func (s byNodeTicket) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func nodeInfo(n *srvpb.Node) *xpb.NodeInfo {
	ni := &xpb.NodeInfo{Ticket: n.Ticket}
	for _, fact := range n.Fact {
		ni.Fact = append(ni.Fact, &xpb.Fact{Name: fact.Name, Value: fact.Value})
	}
	return ni
}

func makeFactList(keyVals ...string) (facts []*srvpb.Node_Fact) {
	if len(keyVals)%2 != 0 {
		panic("makeFactList: odd number of key values")
	}
	for i := 0; i < len(keyVals); i += 2 {
		facts = append(facts, &srvpb.Node_Fact{
			Name:  keyVals[i],
			Value: []byte(keyVals[i+1]),
		})
	}
	return
}

func edgeSet(kinds []string, pes *srvpb.PagedEdgeSet, pages []*srvpb.EdgePage) *xpb.EdgeSet {
	set := stringset.New(kinds...)

	es := &xpb.EdgeSet{
		SourceTicket: pes.EdgeSet.SourceTicket,
	}
	for _, g := range pes.EdgeSet.Group {
		if set.Contains(g.Kind) || len(set) == 0 {
			es.Group = append(es.Group, &xpb.EdgeSet_Group{
				Kind:         g.Kind,
				TargetTicket: g.TargetTicket,
			})
		}
	}
	for _, ep := range pages {
		g := ep.EdgesGroup
		if set.Contains(g.Kind) || len(set) == 0 {
			es.Group = append(es.Group, &xpb.EdgeSet_Group{
				Kind:         g.Kind,
				TargetTicket: g.TargetTicket,
			})
		}
	}
	return es
}

func refs(ds []*srvpb.FileDecorations_Decoration) (refs []*xpb.DecorationsReply_Reference) {
	for _, d := range ds {
		refs = append(refs, &xpb.DecorationsReply_Reference{
			SourceTicket: d.Anchor.Ticket,
			TargetTicket: d.TargetTicket,
			Kind:         d.Kind,
		})
	}
	return
}

type testTable struct {
	Nodes       []*srvpb.Node
	EdgePages   []*srvpb.EdgePage
	EdgeSets    []*srvpb.PagedEdgeSet
	Decorations []*srvpb.FileDecorations
}

func (tbl *testTable) Construct(t *testing.T) *Table {
	p := make(testProtoTable)
	for _, n := range tbl.Nodes {
		testutil.FatalOnErrT(t, "Error writing node: %v", p.Put(NodeKey(n.Ticket), n))
	}
	for _, es := range tbl.EdgeSets {
		testutil.FatalOnErrT(t, "Error writing edge set: %v", p.Put(EdgeSetKey(es.EdgeSet.SourceTicket), es))
	}
	for _, ep := range tbl.EdgePages {
		testutil.FatalOnErrT(t, "Error writing edge page: %v", p.Put([]byte(edgePagesTablePrefix+ep.PageKey), ep))
	}
	for _, d := range tbl.Decorations {
		testutil.FatalOnErrT(t, "Error writing file decorations: %v", p.Put(DecorationsKey(d.FileTicket), d))
	}
	return &Table{p}
}

type testProtoTable map[string]proto.Message

func (t testProtoTable) Put(key []byte, val proto.Message) error { t[string(key)] = val; return nil }

func (t testProtoTable) Lookup(key []byte, msg proto.Message) error {
	m, ok := t[string(key)]
	if !ok {
		log.Println("Failed to find key", string(key))
		return table.ErrNoSuchKey
	}
	proto.Merge(msg, m)
	return nil
}

func (t testProtoTable) Close() error { return nil }