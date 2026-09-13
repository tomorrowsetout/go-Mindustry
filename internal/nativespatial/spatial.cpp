#include "spatial.h"

#include <cmath>
#include <cstdint>
#include <cstring>
#include <unordered_map>
#include <vector>

struct ns_grid {
    int cell_size = 64;
    int n = 0;
    // CSR: cell_start[c] .. cell_start[c+1] holds entity indices for hashed cell c.
    std::unordered_map<int64_t, int> cell_slot;
    std::vector<int> cell_start;
    std::vector<int> items;
    std::vector<int> scratch;
};

static inline int64_t pack_cell(int cx, int cy) {
    return (static_cast<int64_t>(static_cast<uint32_t>(cx)) << 32) |
           static_cast<uint32_t>(cy);
}

static inline int floor_div(float v, int cell) {
    return static_cast<int>(std::floor(static_cast<double>(v) / static_cast<double>(cell)));
}

extern "C" {

ns_grid* ns_grid_create(int cell_size) {
    if (cell_size <= 0) cell_size = 64;
    auto* g = new ns_grid();
    g->cell_size = cell_size;
    return g;
}

void ns_grid_destroy(ns_grid* g) {
    delete g;
}

void ns_grid_clear(ns_grid* g) {
    if (!g) return;
    g->n = 0;
    g->cell_slot.clear();
    g->cell_start.clear();
    g->items.clear();
}

int ns_grid_len(const ns_grid* g) {
    return g ? g->n : 0;
}

void ns_grid_build(ns_grid* g, const float* xs, const float* ys, int n) {
    if (!g) return;
    if (!xs || !ys || n <= 0) {
        ns_grid_clear(g);
        return;
    }
    g->n = n;
    g->cell_slot.clear();
    g->cell_start.clear();
    g->items.assign(n, 0);

    const int cell = g->cell_size;
    // Pass 1: unique cells and counts.
    std::vector<int> keys;
    keys.reserve(n);
    std::vector<int> counts;
    counts.reserve(n);
    for (int i = 0; i < n; ++i) {
        const int64_t key = pack_cell(floor_div(xs[i], cell), floor_div(ys[i], cell));
        auto it = g->cell_slot.find(key);
        if (it == g->cell_slot.end()) {
            const int slot = static_cast<int>(keys.size());
            g->cell_slot.emplace(key, slot);
            keys.push_back(0);
            counts.push_back(1);
        } else {
            counts[it->second] += 1;
        }
    }

    // Pass 2: prefix sums (CSR starts, size = cells+1).
    const int cells = static_cast<int>(counts.size());
    g->cell_start.assign(cells + 1, 0);
    for (int c = 0; c < cells; ++c) {
        g->cell_start[c + 1] = g->cell_start[c] + counts[c];
    }
    std::vector<int> cursor(g->cell_start.begin(), g->cell_start.begin() + cells);

    // Pass 3: scatter indices.
    for (int i = 0; i < n; ++i) {
        const int64_t key = pack_cell(floor_div(xs[i], cell), floor_div(ys[i], cell));
        const int slot = g->cell_slot[key];
        g->items[cursor[slot]++] = i;
    }
}

int ns_grid_query_aabb(ns_grid* g, float min_x, float min_y, float max_x, float max_y, int* out, int max_out) {
    if (!g || !out || max_out <= 0 || g->n == 0 || g->cell_start.empty()) return 0;
    const int cell = g->cell_size;
    const int min_cx = floor_div(min_x, cell);
    const int max_cx = floor_div(max_x, cell);
    const int min_cy = floor_div(min_y, cell);
    const int max_cy = floor_div(max_y, cell);
    int written = 0;
    for (int cy = min_cy; cy <= max_cy; ++cy) {
        for (int cx = min_cx; cx <= max_cx; ++cx) {
            auto it = g->cell_slot.find(pack_cell(cx, cy));
            if (it == g->cell_slot.end()) continue;
            const int slot = it->second;
            const int start = g->cell_start[slot];
            const int end = g->cell_start[slot + 1];
            for (int k = start; k < end; ++k) {
                out[written++] = g->items[k];
                if (written >= max_out) return written;
            }
        }
    }
    return written;
}

}  // extern "C"
