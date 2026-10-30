#import <Foundation/Foundation.h>
// Explicit error convention keeps gomobile's generated nonnull string return
// types from changing the Swift signature between toolchain versions.
FOUNDATION_EXPORT NSString * _Nullable NaiLuEngineConfiguration(NSString * _Nonnull input, NSError * _Nullable * _Nullable error) __attribute__((swift_error(none)));
FOUNDATION_EXPORT NSString * _Nullable NaiLuEngineParseSubscription(NSString * _Nonnull input, NSError * _Nullable * _Nullable error) __attribute__((swift_error(none)));
FOUNDATION_EXPORT BOOL NaiLuEngineStart(NSString * _Nonnull input, NSError * _Nullable * _Nullable error) __attribute__((swift_error(none)));
FOUNDATION_EXPORT void NaiLuEngineStop(void);
