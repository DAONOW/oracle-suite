package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/makerdao/gofer/pkg/graph"
)

type JSON struct {
	Origins    JSONOrigin     `json:"origin"`
	Aggregator JSONAggregator `json:"aggregator"`
}

type JSONOrigin struct {
	Type   string      `json:"type"`
	Name   string      `json:"name"`
	Config interface{} `json:"config"`
}

type JSONAggregator struct {
	Name       string         `json:"name"`
	Parameters JSONParameters `json:"parameters"`
}

type JSONParameters struct {
	PriceModels map[string]JSONPriceModel `json:"pricemodels"`
}

type JSONPriceModel struct {
	Method           string          `json:"method"`
	MinSourceSuccess int             `json:"minSourceSuccess"`
	Sources          [][]JSONSources `json:"sources"`
}

type JSONSources struct {
	Origin string `json:"origin"`
	Pair   string `json:"pair"`
}

func ParseJSONFile(path string) (*JSON, error) {
	f, err := os.Open(path)
	defer f.Close()

	if err != nil {
		return nil, fmt.Errorf("failed to load json config file: %w", err)
	}

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to load json config file: %w", err)
	}

	return ParseJSON(b)
}

func ParseJSON(b []byte) (*JSON, error) {
	j := &JSON{}
	err := json.Unmarshal(b, j)
	if err != nil {
		return nil, err
	}

	return j, nil
}

func (j *JSON) BuildGraphs() (map[graph.Pair]graph.Aggregator, error) {
	graphs := map[graph.Pair]graph.Aggregator{}

	// Build roots. It's important to create root nodes before branches,
	// because branches may refer to another root nodes.
	for name, model := range j.Aggregator.Parameters.PriceModels {
		pair, err := graph.NewPair(name)
		if err != nil {
			return nil, err
		}

		switch model.Method {
		case "median":
			graphs[pair] = graph.NewMedianAggregatorNode(pair, model.MinSourceSuccess)
		default:
			return nil, fmt.Errorf("unknown method: %s", model.Method)
		}
	}

	// Build branches.
	for name, model := range j.Aggregator.Parameters.PriceModels {
		pair, _ := graph.NewPair(name)
		for _, sources := range model.Sources {
			var children []graph.Node
			for _, source := range sources {
				sourcePair, err := graph.NewPair(source.Pair)
				if err != nil {
					return nil, err
				}

				if source.Origin == "." {
					// The reference to an other root node:
					children = append(children, graphs[sourcePair].(graph.Node))
				} else {
					// The origin node:
					pair, err := graph.NewPair(source.Pair)
					if err != nil {
						return nil, err
					}

					originPair := graph.OriginPair{
						Origin: source.Origin,
						Pair:   pair,
					}

					children = append(children, graph.NewOriginNode(originPair))
				}
			}

			var node graph.Node
			if len(children) == 1 {
				// If there is only one node, there is no need to wrap it with
				// IndirectAggregatorNode.
				node = children[0]
			} else {
				indirectAggregator := graph.NewIndirectAggregatorNode(pair)
				for _, c := range children {
					indirectAggregator.AddChild(c)
				}
				node = indirectAggregator
			}

			if typedNode, ok := graphs[pair].(graph.Parent); ok {
				typedNode.AddChild(node)
			}
		}
	}

	return graphs, nil
}
