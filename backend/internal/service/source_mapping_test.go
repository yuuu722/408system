package service

import("testing";"math";"github.com/local/exam408-assistant/backend/internal/domain";"github.com/stretchr/testify/require")

func TestCanonicalSourceMapping(t *testing.T){
 page:=TextPage{Number:2,CoordinateSpace:"normalized-top-left-displayed-page",Blocks:[]domain.DocumentBlock{{ID:"a",Text:"同词😀同词",Kind:"text",Box:[]float64{0,.1,1,.2}},{ID:"b",Text:"第二处同词",Kind:"text",Box:[]float64{0,.3,1,.4}}}}
 CanonicalPage(&page);require.Equal(t,"同词😀同词\n第二处同词",page.Text);require.Equal(t,6,page.Blocks[1].Start)
 chunks:=MappedChunks(page,"revision","space");require.Len(t,chunks,1);regions:=chunks[0].Metadata["regions"].([]domain.SourceRegion);require.Len(t,regions,2);require.NotEqual(t,regions[0].BlockID,regions[1].BlockID);require.Equal(t,6,regions[1].Start)
 page.CoordinateSpace="unsupported-crop";regions=MappedChunks(page,"r","s")[0].Metadata["regions"].([]domain.SourceRegion);require.Empty(t,regions[0].Box)
 for _,vectors:=range [][][]float32{{{0,0}},{{float32(math.NaN()),1}},{{1},{1,2}},{{}}}{require.Error(t,validateVectors(vectors,len(vectors)))}
}
