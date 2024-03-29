package noop

import (
	dgrblock "github.com/ribasushi/DAGger/internal/dagger/block"
	dgrcollector "github.com/ribasushi/DAGger/internal/dagger/collector"
)

type nulCollector struct{ *dgrcollector.DaggerConfig }

func (*nulCollector) AppendBlock(*dgrblock.Header) { return }
func (*nulCollector) FlushState() *dgrblock.Header { return nil }
func (nc *nulCollector) AppendData(ds dgrblock.DataSource) *dgrblock.Header {
	return nc.NodeEncoder.NewLeaf(ds)
}
