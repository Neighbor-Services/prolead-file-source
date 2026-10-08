import 'dart:async';
import 'dart:math';

/// Retry policy with exponential backoff and jitter for network requests.
class ProleadRetryPolicy {
  final int maxRetries;
  final Duration initialDelay;
  final Duration maxDelay;
  final double backoffMultiplier;
  final bool Function(Object error)? retryIf;

  const ProleadRetryPolicy({
    this.maxRetries = 3,
    this.initialDelay = const Duration(milliseconds: 500),
    this.maxDelay = const Duration(seconds: 10),
    this.backoffMultiplier = 2.0,
    this.retryIf,
  });

  /// Execute an asynchronous operation with retry logic.
  Future<T> execute<T>(Future<T> Function() action) async {
    int attempts = 0;
    Duration delay = initialDelay;
    final random = Random();

    while (true) {
      try {
        attempts++;
        return await action();
      } catch (error) {
        if (attempts > maxRetries) {
          rethrow;
        }

        if (retryIf != null && !retryIf!(error)) {
          rethrow;
        }

        // Add full jitter (0 to 100ms)
        final jitter = Duration(milliseconds: random.nextInt(100));
        final waitTime = delay + jitter;
        await Future.delayed(waitTime);

        // Exponential backoff
        final nextMs = (delay.inMilliseconds * backoffMultiplier).toInt();
        delay = Duration(milliseconds: min(nextMs, maxDelay.inMilliseconds));
      }
    }
  }
}
