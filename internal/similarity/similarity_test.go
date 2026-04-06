package similarity

import "testing"

func TestCosineSimilarity(t *testing.T) {
	identical := CosineSimilarity([]float64{1, 2, 3}, []float64{1, 2, 3})
	if identical < 0.999 {
		t.Fatalf("expected identical vectors to be nearly 1, got %.4f", identical)
	}

	different := CosineSimilarity([]float64{1, 0}, []float64{0, 1})
	if different > 0.001 {
		t.Fatalf("expected orthogonal vectors to be near 0, got %.4f", different)
	}
}

func TestCentroid(t *testing.T) {
	centroid := Centroid([][]float64{
		{1, 0, 1},
		{0, 1, 1},
	})

	expected := []float64{0.5, 0.5, 1}
	for index := range expected {
		if centroid[index] != expected[index] {
			t.Fatalf("expected centroid[%d]=%.2f, got %.2f", index, expected[index], centroid[index])
		}
	}
}
