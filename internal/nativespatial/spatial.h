#pragma once

#ifdef __cplusplus
extern "C" {
#endif

typedef struct ns_grid ns_grid;

ns_grid* ns_grid_create(int cell_size);
void ns_grid_destroy(ns_grid* g);
void ns_grid_clear(ns_grid* g);
int ns_grid_len(const ns_grid* g);
void ns_grid_build(ns_grid* g, const float* xs, const float* ys, int n);
int ns_grid_query_aabb(ns_grid* g, float min_x, float min_y, float max_x, float max_y, int* out, int max_out);

#ifdef __cplusplus
}
#endif
