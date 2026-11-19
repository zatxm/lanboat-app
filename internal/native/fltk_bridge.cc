//go:build linux && fltk

#include "fltk_bridge.h"
#include <FL/Fl.H>
#include <FL/Fl_Image.H>
#include <FL/Fl_Double_Window.H>
#include <FL/Fl_Group.H>
#include <FL/Fl_Box.H>
#include <FL/Fl_Button.H>
#include <FL/Fl_Input.H>
#include <FL/Fl_Text_Editor.H>
#include <FL/Fl_Text_Buffer.H>
#include <FL/Fl_Choice.H>
#include <FL/Fl_File_Chooser.H>
#include <FL/Fl_Scroll.H>
#include <FL/Fl_Menu_Window.H>
#include <FL/Fl_Tooltip.H>
#include <FL/platform.H>
#include <FL/x11.H>
#include <X11/Xatom.h>
#include <X11/Xutil.h>
#include <FL/fl_draw.H>
#include <fontconfig/fontconfig.h>
#include <algorithm>
#include <cmath>
#include <atomic>
#include <cstdio>
#include <cstring>
#include <cstdlib>
#include <string>
#include <string_view>
#include <vector>
#include <map>
#include <memory>

extern "C" void goEvent(int, char *, char *);
extern "C" int goIdle(void);

extern "C" const char *lb_choose_file(const char *title, const char *pattern) {
    return fl_file_chooser(title, pattern, nullptr);
}
namespace {
Fl_Double_Window *window = nullptr;
Fl_Double_Window *dialog_window = nullptr;
Fl_Window *dialog_backdrop = nullptr;
LBWidget *dialog_root = nullptr;
LBWidget *root = nullptr;
bool layout_dirty = false;
std::atomic<bool> wake_pending{false};
bool dark_theme=false;
Fl_Widget *back_button=nullptr;
// A persistent, non-interactive leaf keeps FLTK from moving focus to an input.
bool keyboard_focus=false;
Fl_Color ink=fl_rgb_color(31,35,41), muted=fl_rgb_color(141,145,153),
    blue=fl_rgb_color(37,116,185), background=FL_WHITE,
    surface=FL_WHITE, line=fl_rgb_color(237,238,241);
struct Insets { int top=0,right=0,bottom=0,left=0; };
struct Style {
    Insets pad, margin;
    Fl_Color fg=ink, bg=background, border=line;
    bool fill=false, stroke=false, bold=false;
    int font=12, radius=0, minw=0, minh=0;
};
bool has(const std::string &s, const char *name) {
    std::string_view text(s),token(name);size_t pos=0;
    while((pos=text.find(token,pos))!=std::string_view::npos){
        size_t end=pos+token.size();
        if((pos==0 || text[pos-1]==' ') && (end==text.size() || text[end]==' '))return true;
        pos=end;
    }
    return false;
}
void rounded(int x,int y,int w,int h,int r,Fl_Color c) {
    fl_color(c); r=std::min({r,w/2,h/2});
    if(r<=0){fl_rectf(x,y,w,h);return;}
    fl_rectf(x+r,y,w-2*r,h);fl_rectf(x,y+r,w,h-2*r);
    fl_pie(x,y,2*r,2*r,90,180);fl_pie(x+w-2*r,y,2*r,2*r,0,90);
    fl_pie(x,y+h-2*r,2*r,2*r,180,270);fl_pie(x+w-2*r,y+h-2*r,2*r,2*r,270,360);
}
void rectangle(int x,int y,int w,int h,const Style &s) {
    if(s.stroke){rounded(x,y,w,h,s.radius,s.border);rounded(x+1,y+1,w-2,h-2,std::max(0,s.radius-1),s.bg);}
    else if(s.fill)rounded(x,y,w,h,s.radius,s.bg);
}

void rounded_edge(int x,int y,int w,int h,int radius,Fl_Color outside,Fl_Color border,bool stroke) {
    int r=std::max(0,std::min({radius,w/2,h/2}));
    // Children may fill the whole row; restore the card corners and outline last.
    fl_color(outside);
    for(int dy=0;dy<r;dy++){
        double distance=r-dy-.5;
        int inset=std::max(0,int(std::ceil(r-std::sqrt(r*r-distance*distance)-.5)));
        fl_rectf(x,y+dy,inset,1);fl_rectf(x+w-inset,y+dy,inset,1);
        fl_rectf(x,y+h-1-dy,inset,1);fl_rectf(x+w-inset,y+h-1-dy,inset,1);
    }
    if(!stroke)return;
    fl_color(border);fl_line_style(FL_SOLID,1);
    if(r==0)fl_rect(x,y,w,h);
    else {
        fl_line(x+r,y,x+w-r-1,y);fl_line(x+r,y+h-1,x+w-r-1,y+h-1);
        fl_line(x,y+r,x,y+h-r-1);fl_line(x+w-1,y+r,x+w-1,y+h-r-1);
        fl_arc(x,y,2*r,2*r,90,180);fl_arc(x+w-2*r-1,y,2*r,2*r,0,90);
        fl_arc(x,y+h-2*r-1,2*r,2*r,180,270);fl_arc(x+w-2*r-1,y+h-2*r-1,2*r,2*r,270,360);
    }
    fl_line_style(0);
}
Fl_Color inherited_background(const Fl_Widget *from);

void draw_icon(const std::string &name,int cx,int cy,Fl_Color color,bool enabled=true) {
    if(name=="delete"){
        color=dark_theme?fl_rgb_color(236,134,130):fl_rgb_color(187,56,53);
        if(!enabled)color=fl_color_average(color,background,.6f);
    }
    fl_color(color);fl_line_style(FL_SOLID,1.6);
    auto l=[&](int x1,int y1,int x2,int y2){fl_line(cx+x1,cy+y1,cx+x2,cy+y2);};
    if(name=="home"){l(-10,0,0,-9);l(0,-9,10,0);l(-7,-1,-7,9);l(-7,9,7,9);l(7,9,7,-1);fl_rect(cx-2,cy+2,5,7);}
    else if(name=="folder"){l(-10,-7,-3,-7);l(-3,-7,0,-4);l(0,-4,10,-4);l(10,-4,8,8);l(8,8,-10,8);l(-10,8,-10,-7);l(-10,-1,9,-1);}
    else if(name=="speed"){fl_arc(cx-10,cy-10,20,20,0,180);l(-10,0,-10,5);l(10,0,10,5);l(-10,5,10,5);l(0,1,6,-6);fl_circle(cx,cy+1,2);}
    else if(name=="data" || name=="test"){l(-10,0,-5,0);l(-5,0,-2,-8);l(-2,-8,2,8);l(2,8,5,-3);l(5,-3,7,0);l(7,0,11,0);}
    else if(name=="settings"){fl_circle(cx,cy,7);fl_circle(cx,cy,2.5);for(int i=0;i<8;i++){double a=i*3.14159265/4;l(int(9*cos(a)),int(9*sin(a)),int(12*cos(a)),int(12*sin(a)));}}
    else if(name=="back" || name=="forward"){int k=name=="back"?1:-1;l(-1*k,-7,-8*k,0);l(-8*k,0,-1*k,7);l(-8*k,0,9*k,0);}
    else if(name=="add"){l(0,-8,0,8);l(-8,0,8,0);}
    else if(name=="close"){l(-6,-6,6,6);l(-6,6,6,-6);}
    else if(name=="check"){l(-7,0,-2,5);l(-2,5,8,-6);}
    else if(name=="import" || name=="export" || name=="download" || name=="upload"){
        if(name=="upload"){l(0,4,0,-9);l(-4,-5,0,-9);l(0,-9,4,-5);}
        else {l(0,-9,0,4);l(-4,0,0,4);l(0,4,4,0);}
        l(-8,3,-8,9);l(-8,9,8,9);l(8,9,8,3);
    }
    else if(name=="share"){l(-4,-1,3,-5);l(-4,1,3,5);fl_circle(cx-6,cy,2);fl_circle(cx+6,cy-6,2);fl_circle(cx+6,cy+6,2);}
    else if(name=="delete"){l(-9,-5,9,-5);l(-6,-5,-5,9);l(-5,9,5,9);l(5,9,6,-5);l(-3,-8,3,-8);l(-3,-8,-3,-5);l(3,-8,3,-5);l(-2,-1,-2,6);l(2,-1,2,6);}
    else if(name=="edit"){l(-8,4,4,-8);l(4,-8,8,-4);l(8,-4,-4,8);l(-4,8,-8,9);l(-8,9,-8,4);l(1,-5,5,-1);}
    else if(name=="save"){fl_rect(cx-8,cy-9,16,18);fl_rect(cx-4,cy-9,8,6);fl_rect(cx-4,cy+2,8,7);}
    else if(name=="language"){l(-8,-7,4,-7);l(4,-7,4,2);l(4,2,-3,2);l(-3,2,-7,5);l(-7,5,-7,2);l(-7,2,-8,2);l(-8,2,-8,-7);l(-6,0,-2,-5);l(-2,-5,2,0);l(-4,-2,0,-2);l(7,-2,7,7);l(7,7,2,7);}
    else if(name=="shield"){l(0,-8,7,-5);l(7,-5,6,2);l(6,2,0,8);l(0,8,-6,2);l(-6,2,-7,-5);l(-7,-5,0,-8);l(-3,0,-1,2);l(-1,2,3,-2);}
    else if(name=="copy"){fl_rect(cx-5,cy-5,13,15);l(-8,6,-10,6);l(-10,6,-10,-10);l(-10,-10,3,-10);l(3,-10,3,-7);}
    else if(name=="refresh"){fl_arc(cx-8,cy-8,16,16,40,310);l(8,-8,8,-2);l(8,-2,2,-2);}
    else if(name=="link"){fl_arc(cx-10,cy-3,12,12,65,285);fl_arc(cx-2,cy-9,12,12,-115,105);l(-4,4,4,-4);}
    else if(name=="tcp"){l(-10,-4,8,-4);l(8,-4,3,-9);l(8,-4,3,1);l(10,5,-8,5);l(-8,5,-3,0);l(-8,5,-3,10);}
    else if(name=="udp"){fl_circle(cx-7,cy,3);fl_circle(cx+7,cy-6,3);fl_circle(cx+7,cy+6,3);l(-4,0,4,-6);l(-4,0,4,6);}
    else if(name=="route"){fl_circle(cx,cy,9);l(-4,4,-1,-2);l(-1,-2,5,-5);l(5,-5,2,2);l(2,2,-4,4);}
    else if(name=="logs"){fl_rect(cx-7,cy-9,14,18);l(-3,-4,4,-4);l(-3,0,4,0);l(-3,4,2,4);}
    else if(name=="dns" || name=="globe"){fl_circle(cx,cy,9);fl_arc(cx-4,cy-9,8,18,0,360);l(-8,-3,8,-3);l(-8,3,8,3);}
    else if(name=="moon"){fl_arc(cx-8,cy-8,16,16,55,320);fl_arc(cx-1,cy-10,14,14,130,300);}
    else if(name=="fullscreen"){l(-9,-3,-9,-9);l(-9,-9,-3,-9);l(3,-9,9,-9);l(9,-9,9,-3);l(9,3,9,9);l(9,9,3,9);l(-3,9,-9,9);l(-9,9,-9,3);}
    else if(name=="server"){fl_rect(cx-9,cy-9,18,7);fl_rect(cx-9,cy+2,18,7);fl_point(cx-5,cy-5);fl_point(cx-5,cy+5);l(2,-5,5,-5);l(2,5,5,5);}
    else if(name=="layers"){l(-10,-3,0,-9);l(0,-9,10,-3);l(10,-3,0,3);l(0,3,-10,-3);l(-10,2,0,8);l(0,8,10,2);}
    else if(name=="sail"){l(0,-11,0,6);l(-2,-8,-10,4);l(-10,4,-2,4);l(3,-5,9,4);l(9,4,3,4);l(-11,8,9,8);l(-8,8,-4,12);l(-4,12,5,12);l(5,12,9,8);}
    else if(name=="clock"){fl_circle(cx,cy,9);l(0,-5,0,0);l(0,0,4,3);}
    else if(name=="monitor"){fl_rect(cx-10,cy-8,20,14);l(0,6,0,10);l(-6,10,6,10);}
    else if(name=="pie"){fl_arc(cx-9,cy-9,18,18,100,450);l(0,-9,0,0);l(0,0,9,0);}
    else if(name=="down"){l(0,-8,0,8);l(-5,3,0,8);l(0,8,5,3);}
    else if(name=="up"){l(0,8,0,-8);l(-5,-3,0,-8);l(0,-8,5,-3);}
    else if(name=="search"){fl_circle(cx-2,cy-2,6);l(2,2,8,8);}
    else if(name=="info"){fl_circle(cx,cy,9);l(0,-1,0,5);fl_point(cx,cy-5);}
    else {l(-3,-5,2,0);l(2,0,-3,5);}
    fl_line_style(0);
}
std::string icon_of(const std::string &classes){
    size_t start=0;
    while((start=classes.find("icon-",start))!=std::string::npos){
        size_t end=classes.find(' ',start);
        std::string name=classes.substr(start+5,end==std::string::npos?end:end-start-5);
        if(name!="only")return name;
        start=end==std::string::npos?classes.size():end+1;
    }
    return "";
}
class Activity {
    Fl_Widget *owner;
    static void tick(void *data){auto *a=static_cast<Activity*>(data);a->phase=(a->phase+1)%12;a->owner->redraw();Fl::repeat_timeout(.08,tick,a);}
public:
    bool loading=false;int phase=0;
    explicit Activity(Fl_Widget *w):owner(w){}
    virtual ~Activity(){Fl::remove_timeout(tick,this);}
    void set_loading(bool value){if(loading==value)return;loading=value;phase=0;if(value)Fl::add_timeout(.08,tick,this);else Fl::remove_timeout(tick,this);owner->redraw();}
    void spinner(int cx,int cy,Fl_Color color){
        for(int i=0;i<12;i++){double angle=(i+phase)*3.14159265/6;fl_color(fl_color_average(color,surface,float(i+1)/12));fl_line_style(FL_SOLID,2);fl_line(cx+int(5*cos(angle)),cy+int(5*sin(angle)),cx+int(8*cos(angle)),cy+int(8*sin(angle)));}fl_line_style(0);
    }
};
class Button : public Fl_Button, public Activity {
public:
    Style style;std::string classes;
    Button():Fl_Button(0,0,1,1),Activity(this){}
    int handle(int event) override {if(event==FL_ENTER || event==FL_LEAVE){if(window())window()->cursor(event==FL_ENTER && active_r()?(has(classes,"search-field")?FL_CURSOR_INSERT:FL_CURSOR_HAND):FL_CURSOR_DEFAULT);redraw();}return Fl_Button::handle(event);}
    void draw() override {
        Style s=style;
        if(s.bg==surface && !has(classes,"primary"))s.bg=inherited_background(parent()->parent());
        if(!active_r())s.fg=fl_color_average(s.fg,background,.4f);
        else if(!has(classes,"switch") && (value() || Fl::belowmouse()==this))s.bg=fl_color_average(s.bg,ink,.94f);
        if(!has(classes,"switch"))rectangle(x(),y(),w(),h(),s);
        fl_color(s.fg);fl_font(s.bold?17:16,s.font);
        if(has(classes,"icon-only")){if(loading)spinner(x()+w()/2,y()+h()/2,blue);else draw_icon(icon_of(classes),x()+w()/2,y()+h()/2,s.fg,active_r());}
        else if(loading && has(classes,"switch")){spinner(x()+w()/2,y()+h()/2,blue);}
        else if(has(classes,"switch")){bool on=has(classes,"switch-on");int xx=x()+(w()-48)/2,yy=y()+(h()-28)/2;rounded(xx,yy,48,28,14,on?blue:fl_color_average(muted,surface,.45f));fl_color(FL_WHITE);fl_pie(xx+(on?23:3),yy+3,22,22,0,360);}
        else {
            std::string icon=loading?"activity":icon_of(classes),text=label()?label():"";
            bool left=has(classes,"row-button") || has(classes,"nav-item") || has(classes,"header-back") || has(classes,"flush-left");
            int iconWidth=icon.empty()?0:(has(classes,"settings-align")?40:(has(classes,"tight-icon")?22:28));
            if(!icon.empty() && has(classes,"nav-item"))iconWidth=64;
            int textWidth=int(fl_width(text.c_str()))+iconWidth;
            int xx=has(classes,"header-back")?x()-2:(left?x()+s.pad.left:x()+std::max(has(classes,"home-action")?0:10,(w()-textWidth)/2));
            if(loading)spinner(xx+10,y()+h()/2,s.fg);else if(!icon.empty())draw_icon(icon,xx+(has(classes,"settings-align")?14:10),y()+h()/2,has(classes,"settings-align")?blue:s.fg,active_r());
            int tx=xx+iconWidth,tw=std::max(1,w()-(tx-x())-6);
            fl_color(s.fg);fl_font(s.bold?17:16,s.font);
            if(has(classes,"route-option")){
                auto cut=text.find('\n');std::string title=text.substr(0,cut),help=cut==std::string::npos?"":text.substr(cut+1);
                fl_draw(title.c_str(),tx,y()+13,tw-35,25,FL_ALIGN_LEFT|FL_ALIGN_INSIDE);
                fl_color(muted);fl_font(16,12);fl_draw(help.c_str(),tx,y()+40,tw-35,22,FL_ALIGN_LEFT|FL_ALIGN_INSIDE);
                if(has(classes,"checked"))draw_icon("check",x()+w()-25,y()+h()/2,blue);
            } else if(has(classes,"action-row")){
                auto cut=text.find('\t');std::string title=text.substr(0,cut),value=cut==std::string::npos?"":text.substr(cut+1);
                int title_width=int(std::ceil(fl_width(title.c_str()))),value_width=int(std::ceil(fl_width(value.c_str())));
                if(value.empty())fl_draw(title.c_str(),tx,y(),std::max(1,tw),h(),FL_ALIGN_LEFT|FL_ALIGN_INSIDE|FL_ALIGN_CLIP);
                else if(title_width+value_width+32<=tw){
                    fl_draw(title.c_str(),tx,y(),title_width+4,h(),FL_ALIGN_LEFT|FL_ALIGN_INSIDE|FL_ALIGN_CLIP);
                    fl_color(muted);fl_font(16,13);fl_draw(value.c_str(),tx+title_width+20,y(),value_width+8,h(),FL_ALIGN_LEFT|FL_ALIGN_INSIDE|FL_ALIGN_CLIP);
                } else {
                    int right=std::min(tw/2,value_width+24),left=std::max(1,tw-right-20);
                    fl_draw(title.c_str(),tx,y(),left,h(),FL_ALIGN_LEFT|FL_ALIGN_INSIDE|FL_ALIGN_CLIP);
                    fl_color(muted);fl_font(16,13);fl_draw(value.c_str(),tx+tw-right-15,y(),std::max(1,right),h(),FL_ALIGN_RIGHT|FL_ALIGN_INSIDE|FL_ALIGN_CLIP);
                }
            } else fl_draw(text.c_str(),tx,y(),tw,h(),FL_ALIGN_LEFT|FL_ALIGN_INSIDE|FL_ALIGN_CLIP);
            if(has(classes,"disclosure")){int cx=x()+10,cy=y()+h()/2;fl_color(muted);fl_line_style(FL_SOLID,1.5);if(has(classes,"disclosure-open"))fl_line(cx-3,cy-2,cx,cy+1,cx+3,cy-2);else fl_line(cx-2,cy-3,cx+1,cy,cx-2,cy+3);fl_line_style(0);}
            if(has(classes,"chevron"))draw_icon("chevron",x()+w()-13,y()+h()/2,muted);
        }
        if(!has(classes,"switch") && keyboard_focus && Fl::focus()==this && active_r()){fl_color(blue);fl_line_style(FL_SOLID,1);fl_rect(x()+2,y()+2,w()-4,h()-4);fl_line_style(0);}
    }
};
void apply_popup_shape(Fl_Window *,int);
class ChoicePopup : public Fl_Menu_Window {
public:
    static constexpr int row_height=32,padding=4;
    const Fl_Menu_Item *items;
    int count,top=0,hover,selected=-1,rows,current_index;
    bool dragging=false;
    ChoicePopup(int xx,int yy,int ww,int count_,int visible_rows,const Fl_Menu_Item *items_,int current)
        :Fl_Menu_Window(xx,yy,ww,visible_rows*row_height+2*padding),items(items_),count(count_),
         hover(std::clamp(current,0,count_-1)),rows(visible_rows),current_index(current) {
        border(0);set_modal();end();xclass("lanboat-dropdown");
        top=std::clamp(hover-rows/2,0,std::max(0,count-rows));
        apply_popup_shape(this,8);
    }
    bool contains_root(int rx,int ry) const {
        int px=rx-x_root(),py=ry-y_root();if(px<0 || py<0 || px>=w() || py>=h())return false;
        double dx=std::max({0.,7.5-px,px+.5-(w()-8)}),dy=std::max({0.,7.5-py,py+.5-(h()-8)});
        return dx*dx+dy*dy<=64;
    }
    void draw() override {
        fl_color(surface);fl_rectf(0,0,w(),h());
        fl_push_clip(1,1,w()-2,h()-2);
        for(int i=0;i<rows;i++){
            int index=top+i,yy=padding+i*row_height;
            if(index==hover || index==current_index){
                fl_color(fl_color_average(surface,blue,index==hover?.89f:.95f));fl_rectf(1,yy,w()-2,row_height);
            }
            fl_color(ink);fl_font(16,13);
            fl_draw(items[index].label(),12,yy,w()-48,row_height,FL_ALIGN_LEFT|FL_ALIGN_INSIDE|FL_ALIGN_CLIP);
            if(index==current_index)draw_icon("check",w()-28,yy+row_height/2,blue);
        }
        if(count>rows){
            int track=h()-2*padding,size=std::max(24,track*rows/count),pos=padding+(track-size)*top/(count-rows);
            rounded(w()-7,pos,3,size,2,muted);
        }
        fl_pop_clip();
        rounded_edge(0,0,w(),h(),8,surface,line,true);
    }
    int handle(int event) override {
        if(event==FL_ENTER){cursor(FL_CURSOR_HAND);return 1;}
        if(event==FL_PUSH){
            int rx=Fl::event_x_root(),ry=Fl::event_y_root();
            int px=x_root(),py=y_root();
            if(!contains_root(rx,ry)){
                dragging=false;selected=-1;hide();return 1;
            }
        }
        if((event==FL_PUSH && count>rows && Fl::event_x()>=w()-12 && Fl::event_x()<w() && Fl::event_y()>=0 && Fl::event_y()<h()) || (event==FL_DRAG && dragging)){
            dragging=true;int track=h()-2*padding,size=std::max(24,track*rows/count);
            top=std::clamp((Fl::event_y()-padding-size/2)*(count-rows)/std::max(1,track-size),0,count-rows);redraw();return 1;
        }
        if(event==FL_RELEASE && dragging){dragging=false;return 1;}
        if(event==FL_PUSH){
            if(Fl::event_x()<0 || Fl::event_x()>=w() || Fl::event_y()<0 || Fl::event_y()>=h())hide();
            return 1;
        }
        if(event==FL_MOVE || event==FL_DRAG){
            int i=(Fl::event_y()-padding)/row_height;
            if(Fl::event_y()>=padding && i>=0 && i<rows){hover=top+i;redraw();}return 1;
        }
        if(event==FL_RELEASE){
            int i=(Fl::event_y()-padding)/row_height;
            if(Fl::event_x()>=0 && Fl::event_x()<w() && Fl::event_y()>=padding && i>=0 && i<rows){selected=top+i;hide();}return 1;
        }
        if(event==FL_MOUSEWHEEL){top=std::clamp(top+Fl::event_dy(),0,std::max(0,count-rows));hover=std::clamp(hover,top,top+rows-1);redraw();return 1;}
        if(event==FL_KEYDOWN){
            int key=Fl::event_key();
            if(key==FL_Escape || key==FL_Tab){hide();return 1;}
            if(key==FL_Enter || key==' '){selected=hover;hide();return 1;}
            if(key==FL_Up)hover=std::max(0,hover-1);else if(key==FL_Down)hover=std::min(count-1,hover+1);else if(key==FL_Home)hover=0;else if(key==FL_End)hover=count-1;
            if(hover<top)top=hover;if(hover>=top+rows)top=hover-rows+1;redraw();return 1;
        }
        return Fl_Menu_Window::handle(event);
    }
};
class ChoiceBackdrop : public Fl_Menu_Window {
public:
    ChoiceBackdrop(int xx,int yy,int ww,int hh):Fl_Menu_Window(xx,yy,ww,hh){border(0);end();xclass("lanboat-dropdown-backdrop");}
    void draw() override {fl_color(FL_BLACK);fl_rectf(0,0,w(),h());}
};
bool choice_backdrop_composited(int screen) {
    if(!fl_display)return false;
    char selection[48];std::snprintf(selection,sizeof(selection),"_NET_WM_CM_S%d",screen);
    Atom atom=XInternAtom(fl_display,selection,True);
    return atom!=None && XGetSelectionOwner(fl_display,atom)!=None;
}
void choice_backdrop_opacity(Fl_Window *backdrop) {
    if(!fl_display)return;
    // FLTK has no window opacity API; EWMH lets the desktop compositor blend this mask.
    Atom atom=XInternAtom(fl_display,"_NET_WM_WINDOW_OPACITY",False);
    unsigned long opacity=static_cast<unsigned long>(0xFFFFFFFFUL*0.18);
    XChangeProperty(fl_display,fl_xid(backdrop),atom,XA_CARDINAL,32,PropModeReplace,
                    reinterpret_cast<unsigned char *>(&opacity),1);
    XFlush(fl_display);
}
ChoicePopup *active_choice_popup=nullptr;
ChoiceBackdrop *active_choice_backdrop=nullptr;
Fl_Event_Dispatch previous_choice_dispatch=nullptr;
int choice_popup_dispatch(int event,Fl_Window *window) {
    auto *popup=active_choice_popup;
    if(popup && event==FL_PUSH){
        int px=popup->x_root(),py=popup->y_root();
        int rx=Fl::event_x_root(),ry=Fl::event_y_root();
        if(!popup->contains_root(rx,ry)){
            popup->selected=-1;popup->hide();if(active_choice_backdrop)active_choice_backdrop->hide();return 1;
        }
    }
    return previous_choice_dispatch?previous_choice_dispatch(event,window):Fl::handle_(event,window);
}
class Choice : public Fl_Choice, public Activity {
public:
    Choice():Fl_Choice(0,0,1,1),Activity(this) {box(FL_NO_BOX);}
    void draw() override {
        Style s;s.fill=s.stroke=true;s.bg=surface;s.border=Fl::focus()==this?blue:line;s.radius=9;rectangle(x(),y(),w(),h(),s);
        fl_color(active_r()?ink:muted);fl_font(16,14);const Fl_Menu_Item *item=mvalue();if(item)fl_draw(item->label(),x()+13,y(),w()-45,h(),FL_ALIGN_LEFT|FL_ALIGN_INSIDE|FL_ALIGN_CLIP);
        fl_color(muted);fl_line_style(FL_SOLID,1.5);int xx=x()+w()-22,yy=y()+h()/2;if(loading)spinner(xx,yy,blue);else fl_line(xx-4,yy-2,xx,yy+2,xx+4,yy-2);fl_line_style(0);
    }
    void popup(){
        int count=size()-1;if(count<=0)return;
        int sx,sy,sw,sh;
        int anchor_x=window()->x_root()+x(),anchor_y=window()->y_root()+y();
        int screen=Fl::screen_num(anchor_x+w()/2,anchor_y+h()/2);
        Fl::screen_xywh(sx,sy,sw,sh,screen);
        fl_font(16,13);int width=80;
        for(int i=0;i<count;i++)width=std::max(width,int(std::ceil(fl_width(menu()[i].label())))+60);
        width=std::min(std::max({width,w(),160}),std::max(1,sw-16));
        int below_y=anchor_y+h()+6;
        int below_space=sy+sh-8-below_y;
        int above_space=anchor_y-(sy+8)-6;
        int max_space=std::max(below_space,above_space);
        int rows=std::min({count,12,std::max(1,(max_space-2*ChoicePopup::padding)/ChoicePopup::row_height)});
        int height=rows*ChoicePopup::row_height+2*ChoicePopup::padding;
        bool open_below=below_space>=height || below_space>=above_space;
        int popup_y=open_below?below_y:anchor_y-height-6;
        popup_y=std::clamp(popup_y,sy+8,sy+sh-height-8);
        int popup_x=std::clamp(anchor_x,sx+8,sx+sw-width-8);
        ChoicePopup popup(popup_x,popup_y,width,count,rows,menu(),value());
        ChoiceBackdrop backdrop(sx,sy,sw,sh);Fl_Group::current(nullptr);
        previous_choice_dispatch=Fl::event_dispatch();active_choice_popup=&popup;Fl::event_dispatch(choice_popup_dispatch);
        if(choice_backdrop_composited(screen)){active_choice_backdrop=&backdrop;Fl::grab(&backdrop);backdrop.show();choice_backdrop_opacity(&backdrop);}
        Fl::grab(&popup);popup.show();
        while(popup.shown() && this->window()->shown())Fl::wait();
        Fl::grab(nullptr);active_choice_popup=nullptr;active_choice_backdrop=nullptr;Fl::event_dispatch(previous_choice_dispatch);previous_choice_dispatch=nullptr;
        popup.hide();backdrop.hide();
        if(dialog_window && this->window()==dialog_window){if(dialog_backdrop)dialog_backdrop->show();dialog_window->show();Fl::focus(this);}else Fl::focus(nullptr);
        keyboard_focus=false;redraw();
        if(popup.selected>=0 && popup.selected!=value()){value(popup.selected);do_callback();}
    }
    int handle(int e) override {
        if(e==FL_ENTER || e==FL_LEAVE){if(window())window()->cursor(e==FL_ENTER && active_r()?FL_CURSOR_HAND:FL_CURSOR_DEFAULT);return 1;}
        if(e==FL_PUSH){take_focus();redraw();return 1;}if(e==FL_RELEASE){if(Fl::event_inside(this))popup();return 1;}
        if(e==FL_KEYDOWN && (Fl::event_key()==FL_Enter || Fl::event_key()==' ' || Fl::event_key()==FL_Down)){popup();return 1;}
        if(e==FL_FOCUS || e==FL_UNFOCUS){redraw();return 1;}return 0;
    }
};
class Scroll : public Fl_Scroll {
    bool dragging=false;int drag_offset=0;
    void geometry(int &top,int &size,int &travel,int &range) {
        range=std::max(0,int(scrollbar.maximum()-scrollbar.minimum()));int height=std::max(1,h()-16);
        size=std::min(height,std::max(28,int(height*scrollbar.slider_size())));travel=std::max(1,height-size);
        top=y()+8+(range?int(double(scrollbar.value()-scrollbar.minimum())/range*travel):0);
    }
public:
    Scroll():Fl_Scroll(0,0,1,1){type(VERTICAL);box(FL_NO_BOX);scrollbar_size(12);end();}
    void draw() override {
        Fl_Scroll::draw();
        if(scrollbar.visible()) {fl_push_clip(x()+w()-12,y(),12,h());fl_color(color());fl_rectf(x()+w()-12,y(),12,h());int top,size,travel,range;geometry(top,size,travel,range);rounded(x()+w()-8,top,5,size,3,Fl::belowmouse()==this||dragging?muted:fl_color_average(muted,color(),.55f));fl_pop_clip();}
    }
    int handle(int e) override {
        if(e==FL_MOUSEWHEEL){
            if(!Fl::event_inside(this))return 0;
            for(int i=children()-1;i>=0;i--){auto*c=child(i);if(c!=&scrollbar && c!=&hscrollbar && c->visible() && Fl::event_inside(c) && c->handle(e))return 1;}
            if(!scrollbar.visible() || !Fl::event_dy())return 0;
            int next=std::clamp(yposition()+Fl::event_dy()*36,0,int(scrollbar.maximum()));
            if(next==yposition())return 0;scroll_to(0,next);redraw();return 1;
        }
        if(scrollbar.visible() && (dragging || Fl::event_x()>=x()+w()-12)) {
            int top,size,travel,range;geometry(top,size,travel,range);
            if(e==FL_PUSH){dragging=true;drag_offset=Fl::event_y()>=top&&Fl::event_y()<top+size?Fl::event_y()-top:size/2;}
            if(e==FL_PUSH || e==FL_DRAG){int pos=std::clamp(Fl::event_y()-y()-8-drag_offset,0,travel);scroll_to(0,range?int(double(pos)*range/travel):0);redraw();return 1;}
            if(e==FL_RELEASE){dragging=false;redraw();return 1;}
        }
        return Fl_Scroll::handle(e);
    }
};

}

namespace {
class TextArea : public Fl_Text_Editor {
 Fl_Text_Buffer content;
 bool readonly_,setting=false;
public:
 TextArea(bool readonly):Fl_Text_Editor(0,0,1,1),readonly_(readonly){
  buffer(&content);box(FL_FLAT_BOX);wrap_mode(Fl_Text_Display::WRAP_AT_PIXEL,1);scrollbar_align(0);
  content.add_modify_callback([](int,int added,int removed,int,const char*,void*data){auto*self=static_cast<TextArea*>(data);if(!self->setting && (added||removed))self->do_callback();},this);
 }
 ~TextArea(){buffer(nullptr);}
 void resize(int X,int Y,int W,int H)override{
  bool changed=W!=w();Fl_Text_Editor::resize(X,Y,W,H);
  if(changed){
   // FLTK's large-buffer counter needs a populated viewport before recounting wraps.
   recalc_display();wrap_mode(Fl_Text_Display::WRAP_AT_PIXEL,std::max(1,W-6));
  }
 }
 void value(const char*text){setting=true;content.text(text);insert_position(0);scroll(1,0);setting=false;}
 std::string value(){char*text=content.text();std::string result(text);std::free(text);return result;}
 bool readonly()const{return readonly_;}
 int topLine()const{return mTopLineNum;}
 int handle(int event)override{
  if(event==FL_MOUSEWHEEL){
   if(!Fl::event_inside(this) || !Fl::event_dy())return 0;
   if(display_needs_recalc_){display_needs_recalc_=false;recalc_display();}
   int next=std::clamp(mTopLineNum+Fl::event_dy()*3,1,std::max(1,mNBufferLines+2-mNVisibleLines));
   if(next==mTopLineNum)return 0;scroll(next,0);return 1;
  }
  return readonly_?Fl_Text_Display::handle(event):Fl_Text_Editor::handle(event);
 }
};
}

// Only the widget kinds used by this application cross the C ABI.
struct LBWidget : Fl_Group, Activity {
    std::string kind, classes, text, placeholder;
    std::vector<LBWidget *> nodes;
    std::vector<std::string> option_ids;
    Fl_Widget *inner=nullptr;
    LBWidget *logical_parent=nullptr;
    Style style;
    int id=0,gap=0;
    bool horizontal=false, expand=false, row_pressed=false;
    LBWidget(const char *k,int i):Fl_Group(0,0,1,1),Activity(this),kind(k),id(i) {end();Fl_Group::current(nullptr);}
    void row_click(){char event[]="click",value[]="";goEvent(id,event,value);}
    int handle(int event) override {
        if(!has(classes,"node-row") && !has(classes,"search-field"))return Fl_Group::handle(event);
        if(event==FL_ENTER || event==FL_LEAVE){if(window())window()->cursor(event==FL_ENTER && active_r()?(has(classes,"search-field")?FL_CURSOR_INSERT:FL_CURSOR_HAND):FL_CURSOR_DEFAULT);}
        if(event==FL_PUSH){
            if(Fl_Group::handle(event))return 1;
            if(Fl::event_button()==FL_LEFT_MOUSE && active_r()){
                row_pressed=true;take_focus();return 1;
            }
        }
        if(event==FL_DRAG && row_pressed)return 1;
        if(event==FL_RELEASE && row_pressed){
            row_pressed=false;if(active_r() && Fl::event_inside(this))row_click();return 1;
        }
        if(event==FL_KEYDOWN && Fl::focus()==this && (Fl::event_key()==FL_Enter || Fl::event_key()==' ')){
            row_click();return 1;
        }
        if(event==FL_FOCUS || event==FL_UNFOCUS){redraw();return 1;}
        return Fl_Group::handle(event);
    }
    void theme() {
        Style s;s.fg=ink;s.bg=background;s.border=line;s.font=14;
        if(kind=="button"){s.fill=true;s.bg=surface;s.radius=9;s.pad={8,12,8,12};s.minh=38;s.fg=ink;}
        if(kind=="icon"){s.minw=28;s.minh=28;}
        
        if(kind=="entry"||kind=="select"||kind=="text"){s.fill=s.stroke=true;s.bg=surface;s.radius=10;s.minh=42;s.minw=140;}
        if(has(classes,"compact"))s.minw=80;
        if(has(classes,"connectivity-key"))s.minw=140;
        if(has(classes,"connectivity-title")){s.font=16;s.bold=true;}
        if(kind=="text")s.minh=190;
        if(kind=="qr"){s.minh=248;s.minw=248;}
        if(has(classes,"share-link")){s.minh=38;s.pad={0,10,0,10};s.fill=true;s.bg=surface;s.radius=8;}
        if(has(classes,"dialog-title")){s.font=19;s.bold=true;}
        if(has(classes,"alert-row")){s.fill=true;s.radius=12;s.pad={4,12,4,14};}
        if(has(classes,"alert-text")){s.font=12;}
        if(has(classes,"protocol-choice"))s.minw=240;
        if(has(classes,"dns-select-key"))s.minw=190;
        if(has(classes,"protocol-row")){s.minh=50;s.pad={4,0,4,0};}
        if(kind=="separator"){s.fill=true;s.bg=line;s.minh=1;s.margin={0,18,0,18};}
        if(has(classes,"main"))s.pad={20,30,32,30};
        if(has(classes,"home-controls")){s.minw=220;}
        if(has(classes,"search-field")){s.fill=s.stroke=true;s.bg=surface;s.radius=10;s.pad={4,12,4,12};s.minh=44;}
        if(has(classes,"search-input")){s.bg=surface;s.fill=s.stroke=false;s.radius=0;s.minh=34;s.minw=140;}
        if(has(classes,"header")){s.fill=true;s.bg=background;s.pad={14,42,14,30};s.minh=74;}
        if(has(classes,"sidebar")){s.fill=true;s.bg=dark_theme?fl_rgb_color(24,25,28):FL_WHITE;s.pad={24,14,24,14};s.minw=200;}
        if(has(classes,"brand-subtitle")){s.font=10;s.fg=muted;}
        if(has(classes,"sidebar-caption")){s.font=11;s.fg=muted;s.pad={12,10,0,10};}
        if(has(classes,"nav-item")){s.bg=dark_theme?fl_rgb_color(24,25,28):FL_WHITE;s.minh=49;s.minw=172;s.pad={8,12,8,12};s.font=15;}
        if(has(classes,"toolbar-title")){s.fg=ink;s.font=22;s.bold=true;}
        if(has(classes,"toolbar-brand")){s.fg=FL_WHITE;s.font=19;s.bold=true;s.minw=110;}
        if(has(classes,"toolbar-action")){s.bg=blue;s.fg=FL_WHITE;s.font=23;s.minw=52;s.minh=40;}
        if(has(classes,"tabs-wrap")){s.pad={10,20,12,20};s.fill=true;s.bg=background;}
        if(has(classes,"tabs")){s.fill=s.stroke=true;s.bg=surface;s.radius=31;s.pad={5,5,5,5};}
        if(has(classes,"tab")){s.minh=62;s.minw=90;s.radius=26;s.fg=ink;s.bg=surface;s.pad={0,0,0,0};}
        if(has(classes,"active")){s.bg=dark_theme?fl_rgb_color(56,61,69):fl_rgb_color(234,235,238);s.fg=blue;}
        if(has(classes,"title")){s.font=23;s.bold=true;}
        if(has(classes,"subtitle")){s.font=12;s.fg=muted;}
        if(has(classes,"panel")){s.fill=true;s.stroke=!dark_theme;s.bg=surface;s.radius=18;s.pad={20,22,20,22};}
        if(has(classes,"dialog-surface")){s.fill=true;s.bg=surface;s.pad={20,22,20,22};}
        if(has(classes,"node-editor"))s.pad.right=2;
        if(has(classes,"editor-content"))s.pad.right=20;
        if(has(classes,"group"))s.pad={1,1,1,1};
        if(has(classes,"group-row")){s.minh=66;s.pad={12,18,12,18};}
        if(has(classes,"region-select"))s.minw=170;
        if(has(classes,"rule-update-summary"))s.minw=110;
        if(has(classes,"settings-row")){s.minh=92;s.pad={20,18,20,18};}
        if(has(classes,"connection-title")){s.font=19;s.fg=ink;}
        if(has(classes,"green"))s.fg=fl_rgb_color(40,153,114);
        if(has(classes,"stat")){s.pad={18,18,18,18};s.minw=155;}
        if(has(classes,"stat-value")){s.font=27;s.fg=ink;}
        if(has(classes,"stat-traffic"))s.font=18;
        if(has(classes,"section-title")){s.font=13;s.fg=muted;s.margin={4,14,0,14};}
        if(has(classes,"help")){s.font=12;s.fg=muted;}
        if(has(classes,"about-key"))s.minw=132;
        if(has(classes,"inline-value")){s.font=12;s.minw=160;}
        if(has(classes,"node-list"))s.pad={0,17,0,17};
        if(has(classes,"node-row")){s.pad={14,18,14,18};s.minh=70;}
        if(has(classes,"rule-disabled"))s.fg=muted;
        if(has(classes,"node-selected")){s.bg=dark_theme?fl_rgb_color(42,45,51):fl_rgb_color(245,249,255);s.fill=true;s.radius=0;}
        if(has(classes,"node-name")){s.font=15;s.fg=ink;}
        if(has(classes,"node-delay")){s.minw=84;}
        if(has(classes,"picker-option")){s.font=16;}
        if(has(classes,"row-button")){s.minh=49;s.pad={8,4,8,4};s.minw=70;s.fg=ink;}
        if(has(classes,"link")){s.bg=surface;s.fg=blue;}
        if(has(classes,"flush-left"))s.pad.left=0;
        if(has(classes,"home-action"))s.pad={8,6,8,6};
        if(has(classes,"tiny")){s.font=12;s.pad={4,8,4,8};s.minh=32;}
        if(has(classes,"empty")){s.pad={22,12,22,12};s.font=13;}
        if(has(classes,"proxy-panel"))s.pad={18,20,18,20};
        if(has(classes,"option-group")){s.fill=s.stroke=true;s.bg=dark_theme?fl_rgb_color(34,36,40):fl_rgb_color(250,251,253);s.radius=12;s.pad={16,18,16,18};s.margin={4,0,4,0};}
        if(has(classes,"field"))s.margin={4,0,4,0};
        if(has(classes,"field-label")){s.font=12;s.fg=muted;}
        if(has(classes,"alert")){s.fill=true;s.radius=12;s.pad={12,14,12,14};s.font=12;}
        if(has(classes,"default-dot")){s.fg=fl_rgb_color(255,149,0);}
        if(has(classes,"error")){s.fg=fl_rgb_color(189,76,72);s.bg=dark_theme?fl_rgb_color(62,37,38):fl_rgb_color(255,237,235);}
        if(has(classes,"success")){s.fg=fl_rgb_color(44,138,101);s.bg=dark_theme?fl_rgb_color(31,54,45):fl_rgb_color(235,248,241);}
        if(has(classes,"strategy")){s.font=13;s.fg=blue;}
        if(has(classes,"log")){s.font=12;s.fg=muted;}
        if(has(classes,"primary")){s.bg=s.border=blue;s.fg=dark_theme?background:FL_WHITE;}
        if(has(classes,"toolbar-button")){s.stroke=true;s.minh=44;}
        if(has(classes,"selected")){s.bg=dark_theme?fl_rgb_color(40,43,49):fl_rgb_color(233,237,243);s.fg=ink;}
        if(has(classes,"action-row")){s.minh=64;s.pad={10,18,10,18};s.font=14;s.bg=surface;}
        if(has(classes,"route-option")){s.minh=84;s.pad={8,18,8,18};s.font=16;s.radius=0;}
        if(has(classes,"icon-only")){s.minw=36;s.minh=36;s.pad={0,0,0,0};}
        if(has(classes,"banner-close")){s.minw=28;s.minh=28;}
        if(has(classes,"header-back"))s.pad={8,0,8,0};
        if(has(classes,"danger")){
            s.fg=fl_rgb_color(215,75,70);
            if(has(classes,"primary")){s.fill=true;s.bg=s.border=dark_theme?fl_rgb_color(190,55,52):fl_rgb_color(200,54,50);s.fg=FL_WHITE;}
        }
        if(has(classes,"secondary")){s.fill=true;s.bg=dark_theme?fl_rgb_color(57,61,68):fl_rgb_color(234,237,242);s.fg=ink;}
 if(has(classes,"settings-align")){s.font=15;s.minh=66;s.pad={12,18,12,18};}
 if(has(classes,"backup-tab")){s.minw=110;s.minh=42;}
 if(has(classes,"switch")){s.minw=54;s.minh=34;s.bg=surface;s.fill=s.stroke=false;s.pad={0,0,0,0};}
        style=s;
        if(inner){inner->labelfont(s.bold?17:16);inner->labelsize(s.font);inner->labelcolor(s.fg);
            if(auto *b=dynamic_cast<Button *>(inner)){b->style=s;b->classes=classes;}
            if(auto *input=dynamic_cast<Fl_Input_ *>(inner)){input->textfont(16);input->textsize(14);input->textcolor(ink);input->color(surface);input->selection_color(blue);}
            if(auto *area=dynamic_cast<TextArea *>(inner)){area->textfont(16);area->textsize(14);area->textcolor(ink);area->color(surface);area->selection_color(blue);area->cursor_color(ink);}
            if(auto *scroll=dynamic_cast<Fl_Scroll *>(inner)){auto bg=has(classes,"editor-scroll")?surface:background;scroll->color(bg);scroll->scrollbar.color(bg);scroll->hscrollbar.color(bg);scroll->redraw();}
            if(auto *choice=dynamic_cast<Fl_Choice *>(inner)){choice->textfont(16);choice->textsize(14);choice->color(surface);}
        }
    }

    int minwidth() {
        fl_font(style.bold?17:16,style.font);
        int width=style.minw;
        if(kind=="button" && (has(classes,"icon-only") || has(classes,"switch")))return style.minw+style.margin.left+style.margin.right;
        if(kind=="select" && has(classes,"compact")){auto *choice=static_cast<Fl_Choice *>(inner);for(int i=0;i<choice->size()-1;i++)width=std::max(width,int(std::ceil(fl_width(choice->menu()[i].label())))+45);}
        if(kind=="label" && has(classes,"tight-label"))width=int(std::ceil(fl_width(text.c_str())))+6;
        else if(kind=="label" && (has(classes,"toolbar-label")))width=int(std::ceil(fl_width(text.c_str())))+6;
        else if(kind=="label" && (has(classes,"inline-value") || has(classes,"node-delay") || has(classes,"rule-update-summary")))width=style.minw;
        else if(kind=="label" || kind=="button")width=std::max(width,(int)fl_width(text.c_str())+(has(classes,"home-action")?0:18)+style.pad.left+style.pad.right+(icon_of(classes).empty()?0:(has(classes,"tight-icon")?22:28)));
        else if(kind=="box"){
            int total=0,count=0;
            for(auto *n:nodes)if(n->visible()){total=horizontal?total+n->minwidth():std::max(total,n->minwidth());count++;}
            if(horizontal)total+=std::max(0,count-1)*gap;
            width=std::max(width,total+style.pad.left+style.pad.right);
        }
        if(kind=="button"&&has(classes,"row-button"))width=std::min(width,400);
        if(has(classes,"final-policy"))width=std::min(width,240);
        return width+style.margin.left+style.margin.right;
    }
    std::vector<int> widths(int width) {
        int fixed=0,flex=0,count=0;
        for(auto *n:nodes)if(n->visible()){count++;if(n->expand)flex++;else fixed+=n->minwidth();}
        int share=std::max(1,(width-fixed-std::max(0,count-1)*gap)/std::max(1,flex));
        std::vector<int> result;
        for(auto *n:nodes)result.push_back(n->visible()?(n->expand?share:n->minwidth()):0);
        return result;
    }
    int minheight(int width) {
        
        
        width=std::max(1,width-style.margin.left-style.margin.right-style.pad.left-style.pad.right);
        int height=0;
        if(kind=="button"){fl_font(style.bold?17:16,style.font);return std::max(style.minh,fl_height()+style.pad.top+style.pad.bottom)+style.margin.top+style.margin.bottom;}
        if(kind=="label"){
            fl_font(style.bold?17:16,style.font);
            if(has(classes,"node-title") || has(classes,"node-delay"))height=fl_height();
            else {int tw=width,th=0;fl_measure(text.c_str(),tw,th,0);height=std::max(height,th);}
        } else if(kind=="box" && has(classes,"stats-grid")) {
            int columns=width>=800?4:2,row=0;int cell=(width-(columns-1)*gap)/columns;
            for(auto*n:nodes)if(n->visible())row=std::max(row,n->minheight(cell));
            int rows=(nodes.size()+columns-1)/columns;height=rows*row+std::max(0,rows-1)*gap;
        } else if(kind=="box") {
            int total=0,count=0;auto sizes=widths(width);
            for(size_t i=0;i<nodes.size();i++)if(nodes[i]->visible()){
                int h=nodes[i]->minheight(horizontal?sizes[i]:width);
                total=horizontal?std::max(total,h):total+h;count++;
            }
            if(!horizontal)total+=std::max(0,count-1)*gap;
            height=std::max(height,total);
        }
        return std::max(style.minh,height+style.pad.top+style.pad.bottom)+style.margin.top+style.margin.bottom;
    }
    void layout(int xx,int yy,int ww,int hh) {
        
        
        xx+=style.margin.left;yy+=style.margin.top;
        ww=std::max(1,ww-style.margin.left-style.margin.right);
        hh=std::max(1,hh-style.margin.top-style.margin.bottom);
        if(has(classes,"list-frame") && !nodes.empty() && !nodes[0]->nodes.empty()){
            int content=nodes[0]->nodes[0]->minheight(std::max(1,ww-style.pad.left-style.pad.right-12));
            hh=std::min(hh,content+style.pad.top+style.pad.bottom);
        }
        Fl_Widget::resize(xx,yy,ww,hh);
        int x0=xx+style.pad.left,y0=yy+style.pad.top;
        int width=std::max(1,ww-style.pad.left-style.pad.right),height=std::max(1,hh-style.pad.top-style.pad.bottom);
        if(this==root){for(auto *n:nodes)n->layout(xx,yy,ww,hh);}
        else if(kind=="scroll"){
            auto *scroll=static_cast<Fl_Scroll *>(inner);scroll->resize(xx,yy,ww,hh);
            for(auto *n:nodes){int content_width=has(classes,"editor-scroll") && n->minheight(ww)<=hh?ww:ww-12;
                int nh=std::max(hh,n->minheight(content_width));int offset=std::min(scroll->yposition(),std::max(0,nh-hh));
                scroll->scroll_to(0,offset);n->layout(xx,yy-offset,content_width,nh);}
        } else if(kind=="box" && has(classes,"stats-grid")) {
            int columns=width>=800?4:2,row=0;int cell=(width-(columns-1)*gap)/columns;
            for(auto*n:nodes)if(n->visible())row=std::max(row,n->minheight(cell));
            for(size_t i=0;i<nodes.size();i++)nodes[i]->layout(x0+(i%columns)*(cell+gap),y0+(i/columns)*(row+gap),cell,row);
        } else if(kind=="box") {
            auto sizes=widths(width);int flex=0,used=0,count=0;
            for(auto *n:nodes)if(n->visible()){used+=n->minheight(width);count++;if(n->expand)flex++;}
            used+=std::max(0,count-1)*gap;
            int extra=std::max(0,height-used)/std::max(1,flex);
            for(size_t i=0;i<nodes.size();i++){
                auto *n=nodes[i];if(!n->visible())continue;
                if(horizontal){n->layout(x0,y0,sizes[i],height);x0+=sizes[i]+gap;}
                else {int nh=n->minheight(width)+(n->expand?extra:0);n->layout(x0,y0,width,nh);y0+=nh+gap;}
            }
        } else if(kind=="button"){inner->resize(xx,yy,ww,hh);}
        else if(inner) {
            int inset=(kind=="entry" || kind=="text") && !has(classes,"search-input")?8:0;
            inner->resize(x0+inset,y0+inset,std::max(1,width-2*inset),std::max(1,height-2*inset));
        }
        init_sizes();
    }
    void draw() override {
        // A child-only update must not erase siblings that draw_children() will skip.
        if(damage() & ~FL_DAMAGE_CHILD){
            if(kind!="button")rectangle(x(),y(),w(),h(),style);
            if(!style.fill && !style.stroke){Fl_Color bg=background;for(auto *p=logical_parent;p;p=p->logical_parent)if(p->style.fill){bg=p->style.bg;break;}fl_color(bg);fl_rectf(x(),y(),w(),h());}
        }
        draw_children();
        if(kind=="box" && style.fill && (style.stroke || style.radius>0))
            rounded_edge(x(),y(),w(),h(),style.radius,inherited_background(parent()),style.border,style.stroke);
        if(loading && has(classes,"node-row"))spinner(x()+9,y()+h()/2,blue);
        if(has(classes,"node-row") && keyboard_focus && Fl::focus()==this && active_r()){
            fl_color(blue);fl_line_style(FL_SOLID,1);fl_rect(x()+2,y()+2,w()-4,h()-4);fl_line_style(0);
        }
        if(kind=="icon")draw_icon(text,x()+w()/2,y()+h()/2,has(classes,"stat-icon")?muted:blue,active_r());
        if(kind=="qr"){
            int size=0;for(char c:text){if(c=='\n')break;size++;}
            if(size>0){int scale=std::max(1,std::min(w(),h())/size),edge=scale*size;
                int left=x()+(w()-edge)/2,top=y()+(h()-edge)/2;
                fl_color(FL_WHITE);fl_rectf(left,top,edge,edge);fl_color(FL_BLACK);
                int row=0,col=0;for(char c:text){if(c=='\n'){row++;col=0;}else{if(c=='1')fl_rectf(left+col*scale,top+row*scale,scale,scale);col++;}}
            }
        }
        if(kind=="entry" && !placeholder.empty() && inner!=Fl::focus()){
            auto *input=static_cast<Fl_Input_ *>(inner);
            if(!*input->value()){fl_color(muted);fl_font(16,11);fl_draw(placeholder.c_str(),x()+10,y(),w()-20,h(),FL_ALIGN_LEFT|FL_ALIGN_INSIDE|FL_ALIGN_CLIP);}
        }
    }
};

namespace {
Fl_Color inherited_background(const Fl_Widget *from) {
    for(auto *p=from;p;p=p->parent()){
        auto *n=dynamic_cast<const LBWidget *>(p);
        if(n && n->style.fill)return n->style.bg;
    }
    return background;
}
constexpr int popup_radius=18;
struct PopupShape {
    Fl_Widget_Tracker tracker;
    std::unique_ptr<Fl_RGB_Image> image;
    int width=0,height=0,radius;
    explicit PopupShape(Fl_Window *w,int r=popup_radius):tracker(w),radius(r){}
    void update(Fl_Window *w) {
        if(width==w->w() && height==w->h())return;
        int ww=w->w(),hh=w->h(),r=std::min({radius,ww/2,hh/2});
        auto *pixels=new unsigned char[ww*hh];std::memset(pixels,255,ww*hh);
        for(int y=0;y<r;y++){
            double d=r-y-.5;
            int inset=std::max(0,int(std::ceil(r-std::sqrt(r*r-d*d)-.5)));
            for(int row:{y,hh-1-y}){std::memset(pixels+row*ww,0,inset);std::memset(pixels+row*ww+ww-inset,0,inset);}
        }
        auto next=std::make_unique<Fl_RGB_Image>(pixels,ww,hh,1);next->alloc_array=1;
        w->shape(next.get());image=std::move(next);width=ww;height=hh;w->redraw();
    }
};
// FLTK retains the mask image; trackers release it only after its window is deleted.
std::map<Fl_Window *,std::unique_ptr<PopupShape>> popup_shapes;
void clean_popup_shapes(){
    for(auto it=popup_shapes.begin();it!=popup_shapes.end();){
        if(it->second->tracker.deleted())it=popup_shapes.erase(it);else ++it;
    }
}
void apply_popup_shape(Fl_Window *w,int radius){
    clean_popup_shapes();
    auto &mask=popup_shapes[w];if(!mask)mask=std::make_unique<PopupShape>(w,radius);
    mask->update(w);
}
void shape_popup_windows(){
    clean_popup_shapes();
    std::vector<Fl_Window *> windows;
    for(auto *w=Fl::first_window();w;w=Fl::next_window(w))
        if(w!=window && w!=dialog_backdrop && !w->window() && !dynamic_cast<Fl_Menu_Window *>(w) && !w->menu_window() && !w->tooltip_window())windows.push_back(w);
    for(auto *w:windows){
        auto &mask=popup_shapes[w];if(!mask)mask=std::make_unique<PopupShape>(w);
        mask->update(w);
    }
}
void layout_now(void *) {
    shape_popup_windows();
    if(!layout_dirty || !window || !root)return;
    layout_dirty=false;root->layout(0,0,window->w(),window->h());window->redraw();
    if(dialog_window && dialog_root){dialog_root->layout(0,0,dialog_window->w(),dialog_window->h());dialog_window->redraw();}
}
class MainWindow : public Fl_Double_Window {
public:
    int handle(int e) override {
 // Restoring the owner window after a popup must not pick its first input.
 if(e==FL_FOCUS){if(!contains(Fl::focus()))Fl::focus(this);return 1;}
 if(e==FL_KEYDOWN){if(Fl::event_key()==FL_F+11){if(fullscreen_active())fullscreen_off();else fullscreen();return 1;}
 if(Fl::event_key()==FL_Escape){if(fullscreen_active())fullscreen_off();else if(back_button && back_button->visible_r() && back_button->active_r())back_button->do_callback();return 1;}keyboard_focus=true;}
 else if(e==FL_PUSH){
     keyboard_focus=false;
     auto *focused=Fl::focus();
     if(focused && (dynamic_cast<Fl_Input_*>(focused)||dynamic_cast<TextArea*>(focused)) && !(Fl::event_x()>=focused->x() && Fl::event_x()<focused->x()+focused->w() &&
                     Fl::event_y()>=focused->y() && Fl::event_y()<focused->y()+focused->h()))
         Fl::focus(nullptr);
 }
 int handled=Fl_Double_Window::handle(e);
 if(e==FL_MOVE || e==FL_ENTER){
     Fl_Cursor shape=FL_CURSOR_DEFAULT;
     for(auto *p=Fl::belowmouse();p;p=p->parent()){
         if(!p->active_r())break;
         if((dynamic_cast<Fl_Input_ *>(p)||dynamic_cast<TextArea*>(p))){shape=FL_CURSOR_INSERT;break;}
         auto *group=dynamic_cast<LBWidget *>(p);
         if(dynamic_cast<Fl_Button *>(p) || dynamic_cast<Choice *>(p) || (group && has(group->classes,"node-row"))){shape=FL_CURSOR_HAND;break;}
     }
     cursor(shape);
 }
 return handled;
 }
    MainWindow():Fl_Double_Window(1200,800,"Lanboat"){}
    void resize(int x,int y,int w,int h) override {Fl_Double_Window::resize(x,y,w,h);layout_dirty=true;}
};
class DialogBackdrop : public Fl_Double_Window {
 Fl_RGB_Image *image=nullptr;
public:
 DialogBackdrop(int xx,int yy,int ww,int hh):Fl_Double_Window(xx,yy,ww,hh){border(0);set_override();xclass("lanboat-modal-backdrop");end();
  XImage *capture=XGetImage(fl_display,fl_xid(::window),0,0,ww,hh,AllPlanes,ZPixmap);
  if(capture){auto *pixels=new unsigned char[ww*hh*3];
   for(int y=0;y<hh;y++)for(int x=0;x<ww;x++){unsigned long pixel=XGetPixel(capture,x,y);int i=(y*ww+x)*3;
    pixels[i]=static_cast<unsigned char>(((pixel&capture->red_mask)*178)/capture->red_mask);
    pixels[i+1]=static_cast<unsigned char>(((pixel&capture->green_mask)*178)/capture->green_mask);
    pixels[i+2]=static_cast<unsigned char>(((pixel&capture->blue_mask)*178)/capture->blue_mask);
   }
   XDestroyImage(capture);image=new Fl_RGB_Image(pixels,ww,hh,3);image->alloc_array=1;
  }
 }
 ~DialogBackdrop(){delete image;}
 void draw() override{if(image)image->draw(0,0);else{fl_color(fl_rgb_color(170,170,170));fl_rectf(0,0,w(),h());}}
 int handle(int event) override {return 1;}
};
class DialogWindow : public Fl_Double_Window {
public:
 DialogWindow(int width,int height):Fl_Double_Window(width,height){set_modal();border(0);set_override();xclass("lanboat-dialog");end();}
 void draw() override {Fl_Double_Window::draw();rounded_edge(0,0,w(),h(),popup_radius,surface,line,true);}
 int handle(int event) override {
  if(event==FL_PUSH){auto *focused=Fl::focus();if(focused && (dynamic_cast<Fl_Input_*>(focused)||dynamic_cast<TextArea*>(focused)) && !Fl::event_inside(focused))Fl::focus(nullptr);}
  if(event==FL_KEYDOWN && Fl::event_key()==FL_Escape){if(dialog_root)dialog_root->row_click();return 1;}
  int handled=Fl_Double_Window::handle(event);
  if(event==FL_MOVE || event==FL_ENTER){Fl_Cursor shape=FL_CURSOR_DEFAULT;for(auto*p=Fl::belowmouse();p;p=p->parent()){if(!p->active_r())break;if((dynamic_cast<Fl_Input_ *>(p)||dynamic_cast<TextArea*>(p))){shape=FL_CURSOR_INSERT;break;}if(dynamic_cast<Fl_Button *>(p) || dynamic_cast<Choice *>(p)){shape=FL_CURSOR_HAND;break;}}cursor(shape);}
  return handled;
 }
};
void event_cb(Fl_Widget *,void *data) {
    auto *n=static_cast<LBWidget *>(data);std::string value;
    if(auto *input=dynamic_cast<Fl_Input_ *>(n->inner))value=input->value();
    else if(auto*area=dynamic_cast<TextArea*>(n->inner))value=area->value();
    if(auto *choice=dynamic_cast<Fl_Choice *>(n->inner)){int i=choice->value();if(i>=0 && i<(int)n->option_ids.size())value=n->option_ids[i];char change[]="change";goEvent(n->id,change,const_cast<char *>(value.c_str()));return;}
    char click[]="click",change[]="change";
    goEvent(n->id,n->kind=="button"?click:change,const_cast<char *>(value.c_str()));
}
void awake_cb(void *) {wake_pending=false;goIdle();layout_now(nullptr);}
}

extern "C" {
LBWidget *lb_window(const unsigned char *rgba, int size) {
    if(!std::getenv("DISPLAY"))return nullptr;
    FcInit();Fl::lock();Fl::set_font(16,"Noto Sans CJK SC");Fl::set_font(17,"BNoto Sans CJK SC");Fl_Tooltip::font(16);Fl_Tooltip::size(13);
    Fl::background(255,255,255);Fl::background2(255,255,255);Fl::foreground(31,35,41);
    Fl::scheme("none");window=new MainWindow();window->xclass("lanboat");Fl_RGB_Image icon(rgba,size,size,4);window->icon(&icon);window->size_range(900,640);
    window->end();Fl_Group::current(nullptr);root=new LBWidget("box",0);window->add(root);window->resizable(root);
    Fl::add_check(layout_now);layout_dirty=true;return root;
}
LBWidget *lb_create(const char *kind,int id,const char *text,const char *value,const char *placeholder,
    const char *classes,int horizontal,int gap,int expand,int disabled,int secret,int readonly) {
    auto *n=new LBWidget(kind,id);n->text=text;n->placeholder=placeholder;n->classes=classes;
    n->horizontal=horizontal;n->gap=gap;n->expand=expand;n->begin();
    if(n->kind=="button")n->inner=new Button();
    else if(n->kind=="label"){n->inner=new Fl_Box(0,0,1,1);n->inner->align(((has(n->classes,"node-delay") || has(n->classes,"rule-update-summary"))?FL_ALIGN_RIGHT:FL_ALIGN_LEFT)|FL_ALIGN_INSIDE|FL_ALIGN_CLIP|((has(n->classes,"node-title") || has(n->classes,"node-delay"))?0:FL_ALIGN_WRAP));}
    else if(n->kind=="entry"){auto *i=new Fl_Input(0,0,1,1);if(secret)i->type(FL_SECRET_INPUT);else if(readonly)i->type(FL_NORMAL_OUTPUT);i->value(value);i->box(FL_FLAT_BOX);n->inner=i;}
    else if(n->kind=="text"){auto*i=new TextArea(readonly);i->value(value);n->inner=i;}
    else if(n->kind=="select"){n->inner=new Choice();}
    else if(n->kind=="scroll"){n->inner=new Scroll();}
    n->end();Fl_Group::current(nullptr);
    if(n->inner){if(n->kind=="label" || n->kind=="button")n->inner->copy_label(text);n->inner->callback(event_cb,n);if(has(n->classes,"icon-only"))n->inner->copy_tooltip(text);if(n->kind=="entry" || n->kind=="text")n->inner->when(FL_WHEN_CHANGED);}
    if(has(n->classes,"header-back")){back_button=n->inner;static_cast<Fl_Button *>(n->inner)->shortcut(FL_ALT+FL_Left);}
    n->theme();if(disabled)n->deactivate();return n;
}
const char *lb_fltk_version(void){static char version[32];std::snprintf(version,sizeof(version),"%d.%d.%d",FL_MAJOR_VERSION,FL_MINOR_VERSION,FL_PATCH_VERSION);return version;}
void lb_window_title(const char *text){if(window)window->copy_label(text);}
void lb_placeholder(LBWidget *n,const char *text){n->placeholder=text;n->redraw();}
const char *lb_choose_directory(const char *title){
 static std::string selected;
 Fl_File_Chooser chooser(std::getenv("HOME"),"*",Fl_File_Chooser::DIRECTORY,title);
 chooser.preview(0);
 chooser.show();
 chooser.position(window->x_root()+(window->w()-chooser.w())/2,window->y_root()+(window->h()-chooser.h())/2);
 while(chooser.shown())Fl::wait();
 if(!chooser.value())return nullptr;selected=chooser.value();return selected.c_str();
}
void lb_dialog(LBWidget *content,int width,int height){
 lb_close_dialog();dialog_root=content;
 width=std::min(width,window->w()-40);
 if(height==0)height=std::max(160,content->minheight(width));
 height=std::min(height,window->h()-40);
 layout_now(nullptr);Fl::flush();
 dialog_backdrop=new DialogBackdrop(window->x_root(),window->y_root(),window->w(),window->h());dialog_backdrop->show();
 auto *popup=new DialogWindow(width,height);dialog_window=popup;
 popup->callback([](Fl_Widget *,void *){if(dialog_root)dialog_root->row_click();});
 popup->add(content);popup->resizable(content);popup->position(window->x_root()+(window->w()-width)/2,window->y_root()+(window->h()-height)/2);
 content->layout(0,0,width,height);popup->color(background);apply_popup_shape(popup,popup_radius);popup->show();
 auto first_input=[](auto&&self,LBWidget*n)->Fl_Input_*{if(auto*i=dynamic_cast<Fl_Input_*>(n->inner)){if(!i->readonly())return i;}for(auto*c:n->nodes){if(auto*i=self(self,c))return i;}return nullptr;};
 if(auto*input=first_input(first_input,content)){input->take_focus();input->insert_position(input->size(),0);}
 layout_dirty=true;
}
void lb_fit_dialog(void){
 if(!dialog_window || !dialog_root)return;
 int width=dialog_window->w(),height=std::clamp(dialog_root->minheight(width),160,window->h()-40);
 int y=std::clamp(dialog_window->y(),window->y_root()+20,window->y_root()+window->h()-height-20);
 dialog_window->resize(dialog_window->x(),y,width,height);
 apply_popup_shape(dialog_window,popup_radius);
 if(dialog_backdrop)dialog_backdrop->show();dialog_window->show();
 layout_dirty=true;
}
void lb_close_dialog(void){if(dialog_window){dialog_window->hide();Fl::delete_widget(dialog_window);}dialog_window=nullptr;dialog_root=nullptr;if(dialog_backdrop){dialog_backdrop->hide();Fl::delete_widget(dialog_backdrop);dialog_backdrop=nullptr;}Fl::focus(nullptr);keyboard_focus=false;}
void lb_copy(const char *text){Fl::copy(text,std::strlen(text),1);}
void lb_blur(void){keyboard_focus=false;Fl::focus(nullptr);}
void lb_theme(int dark){dark_theme=dark;ink=dark?fl_rgb_color(229,231,235):fl_rgb_color(31,35,41);muted=dark?fl_rgb_color(151,156,167):fl_rgb_color(115,122,135);blue=dark?fl_rgb_color(184,198,218):fl_rgb_color(75,102,146);background=dark?fl_rgb_color(20,21,24):FL_WHITE;surface=dark?fl_rgb_color(29,30,34):FL_WHITE;line=dark?fl_rgb_color(44,46,52):fl_rgb_color(227,230,235);if(root){auto theme=[](auto&&self,LBWidget *n)->void{n->theme();for(auto*c:n->nodes)self(self,c);};theme(theme,root);if(dialog_root)theme(theme,dialog_root);}if(dialog_window){dialog_window->color(background);dialog_window->redraw();}if(window){window->color(background);window->redraw();}}
void lb_pack(LBWidget *p,LBWidget *c){p->nodes.push_back(c);c->logical_parent=p;if(p->kind=="scroll")static_cast<Fl_Scroll *>(p->inner)->add(c);else p->add(c);layout_dirty=true;}
void lb_clear(LBWidget *p){for(auto *n:p->nodes)delete n;p->nodes.clear();layout_dirty=true;}
void lb_close(void){lb_close_dialog();Fl::remove_check(layout_now);if(window)delete window;clean_popup_shapes();window=nullptr;root=nullptr;back_button=nullptr;layout_dirty=false;}
void lb_show(void){layout_dirty=true;layout_now(nullptr);window->show();}
int lb_run(void){return Fl::run();}
void lb_awake(void){if(!wake_pending.exchange(true))Fl::awake(awake_cb,nullptr);}
void lb_pump(void){Fl::check();layout_now(nullptr);Fl::flush();}
void lb_text(LBWidget *n,const char *text){n->text=text;if(n->inner)n->inner->copy_label(text);n->theme();if(has(n->classes,"toolbar-label") || has(n->classes,"applied-url"))layout_dirty=true;n->redraw();}
void lb_classes(LBWidget *n,const char *classes){n->classes=classes;n->theme();n->redraw();}
void lb_tooltip(LBWidget *n,const char *text){if(n && n->inner)n->inner->copy_tooltip(text);}
void lb_loading(LBWidget *n,int loading){if(n->inner){if(auto*a=dynamic_cast<Activity*>(n->inner))a->set_loading(loading);}else n->set_loading(loading);}
void lb_enabled(LBWidget *n,int enabled){if((bool)n->active()==(bool)enabled)return;if(enabled)n->activate();else {if(n->contains(Fl::focus()))Fl::focus(nullptr);n->deactivate();}if(n->window())n->window()->redraw();else n->redraw();}
void lb_visible(LBWidget *n,int show){if((bool)n->visible()==(bool)show)return;if(show)n->show();else n->hide();layout_dirty=true;}
void lb_expand(LBWidget *n,int expand){n->expand=expand;layout_dirty=true;}
void lb_options(LBWidget *n,const char *options,const char *value){auto *c=static_cast<Fl_Choice *>(n->inner);c->clear();n->option_ids.clear();std::string lines(options);size_t start=0;
    while(start<lines.size()){size_t end=lines.find('\n',start);if(end==std::string::npos)end=lines.size();std::string row=lines.substr(start,end-start);size_t tab=row.find('\t');std::string id=row.substr(0,tab),label=tab==std::string::npos?id:row.substr(tab+1);int index=c->add(("item"+std::to_string(n->option_ids.size())).c_str());c->replace(index,label.c_str());n->option_ids.push_back(id);if(id==value)c->value(n->option_ids.size()-1);start=end+1;}c->redraw();}
void lb_top(LBWidget *n){static_cast<Fl_Scroll *>(n->inner)->scroll_to(0,0);layout_dirty=true;}
int lb_has_focus(LBWidget *n){return n->inner && Fl::focus()==n->inner;}
int lb_scroll_position(LBWidget *n,int position){if(auto*area=dynamic_cast<TextArea*>(n->inner)){if(position>=0)area->scroll(position+1,0);return area->topLine()-1;}auto *scroll=static_cast<Fl_Scroll *>(n->inner);if(position>=0){scroll->scroll_to(0,position);layout_dirty=true;}return scroll->yposition();}
void lb_focus(LBWidget *n){if(n->inner)n->inner->take_focus();}
void lb_click(LBWidget *n){if(!n->active_r())return;if(n->inner)n->inner->do_callback();else if(has(n->classes,"node-row"))n->row_click();}
void lb_edit(LBWidget *n,const char *value){if(auto *i=dynamic_cast<Fl_Input_ *>(n->inner)){i->value(value);i->do_callback();}else if(auto*area=dynamic_cast<TextArea*>(n->inner)){if(!area->readonly()){area->value(value);area->do_callback();}}else if(auto *c=dynamic_cast<Fl_Choice *>(n->inner)){for(size_t j=0;j<n->option_ids.size();j++)if(n->option_ids[j]==value){c->value(j);char change[]="change";goEvent(n->id,change,const_cast<char *>(value));break;}}}
void lb_value(LBWidget*n,const char*value){if(auto*i=dynamic_cast<Fl_Input_*>(n->inner))i->value(value);else if(auto*area=dynamic_cast<TextArea*>(n->inner))area->value(value);}
const char *lb_get_text(LBWidget *n){if(auto *i=dynamic_cast<Fl_Input_ *>(n->inner))return i->value();if(auto*area=dynamic_cast<TextArea*>(n->inner)){n->text=area->value();}return n->text.c_str();}
void lb_bounds(LBWidget *n,int *x,int *y,int *w,int *h){*x=n->x();*y=n->y();*w=n->w();*h=n->h();}
}
