#ifndef LANBOAT_FLTK_BRIDGE_H
#define LANBOAT_FLTK_BRIDGE_H
#ifdef __cplusplus
extern "C" {
#endif
typedef struct LBWidget LBWidget;
LBWidget *lb_window(const unsigned char *rgba, int size);
LBWidget *lb_create(const char *kind, int id, const char *text, const char *value,
                    const char *placeholder, const char *classes, int horizontal,
                    int gap, int expand, int disabled, int secret, int readonly);
void lb_theme(int dark);
void lb_window_title(const char *);
const char *lb_fltk_version(void);
void lb_placeholder(LBWidget *, const char *);
void lb_pack(LBWidget *parent, LBWidget *child);
void lb_clear(LBWidget *parent);
void lb_close(void);
void lb_show(void);
int lb_run(void);
void lb_awake(void);
void lb_pump(void);
void lb_text(LBWidget *, const char *);
void lb_classes(LBWidget *, const char *);
void lb_tooltip(LBWidget *, const char *);
void lb_enabled(LBWidget *, int);
void lb_loading(LBWidget *, int);
void lb_visible(LBWidget *, int);
void lb_expand(LBWidget *, int);
void lb_options(LBWidget *, const char *, const char *);
void lb_top(LBWidget *);
void lb_focus(LBWidget *);
int lb_has_focus(LBWidget *);
const char *lb_choose_file(const char *title, const char *pattern);
const char *lb_choose_directory(const char *title);
void lb_dialog(LBWidget *, int width, int height);
void lb_close_dialog(void);
void lb_fit_dialog(void);
void lb_copy(const char *);
void lb_blur(void);
int lb_scroll_position(LBWidget *, int);
void lb_click(LBWidget *);
void lb_edit(LBWidget *, const char *);
void lb_value(LBWidget *, const char *);
const char *lb_get_text(LBWidget *);
void lb_bounds(LBWidget *, int *x, int *y, int *w, int *h);
#ifdef __cplusplus
}
#endif
#endif
