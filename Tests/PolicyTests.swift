import XCTest
@testable import NaiLu
final class PolicyTests: XCTestCase {
    func testUntrustedSubscriptionRejected() { XCTAssertThrowsError(try NaiLuOrigin.subscription("https://untrusted.invalid/subscribe?token=test")) }
    func testGBFormatting() { XCTAssertEqual(Traffic.gigabytes(1_073_741_824), "1.00 GB") }
}
