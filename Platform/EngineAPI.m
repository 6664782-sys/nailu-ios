#import "EngineAPI.h"
@import NailuEngine;
NSString *NaiLuEngineConfiguration(NSString *input, NSError **error) {
    NSError *failure = nil;
    NSString *result = EngineConfiguration(input, &failure);
    if (error) *error = failure;
    return failure ? nil : result;
}
NSString *NaiLuEngineParseSubscription(NSString *input, NSError **error) {
    NSError *failure = nil;
    NSString *result = EngineParseSubscription(input, &failure);
    if (error) *error = failure;
    return failure ? nil : result;
}
BOOL NaiLuEngineStart(NSString *input, NSError **error) { return EngineStart(input, error); }
void NaiLuEngineStop(void) { NSError *ignored = nil; EngineStop(&ignored); }
