#import <UIKit/UIKit.h>
#import "FegoMobile.h"

extern void LanboatHome(char *, char *, char *);

@interface LanboatScene : UIResponder <UIWindowSceneDelegate>
@property(nonatomic, strong) UIWindow *window;
@end

@implementation LanboatScene
- (void)scene:(UIScene *)scene willConnectToSession:(UISceneSession *)session
                 options:(UISceneConnectionOptions *)options {
    NSURL *dataDir = [NSFileManager.defaultManager URLsForDirectory:NSApplicationSupportDirectory
                                                 inDomains:NSUserDomainMask].firstObject;
    LanboatHome((char *)dataDir.path.UTF8String,
                (char *)NSTimeZone.localTimeZone.name.UTF8String,
                (char *)NSLocale.preferredLanguages.firstObject.UTF8String);
    self.window = [[UIWindow alloc] initWithWindowScene:(UIWindowScene *)scene];
    self.window.rootViewController = FegoMobileViewController();
    [self.window makeKeyAndVisible];
}
@end

@interface LanboatDelegate : UIResponder <UIApplicationDelegate>
@end
@implementation LanboatDelegate
@end

int main(int argc, char **argv) {
    @autoreleasepool {
        return UIApplicationMain(argc, argv, nil, NSStringFromClass(LanboatDelegate.class));
    }
}
