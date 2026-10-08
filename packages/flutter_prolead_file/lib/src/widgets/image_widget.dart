import 'dart:convert';
import 'dart:typed_data';
import 'dart:ui';
import 'package:flutter/material.dart';
import '../models.dart';
import '../client.dart';

/// A reactive Flutter Image Widget optimized for Prolead File with dynamic transforms and LQIP Blur-Up.
class ProleadFileImage extends StatelessWidget {
  final ProleadFileClient client;
  final String rawDownloadUrl;
  final String? lqip;
  final double? width;
  final double? height;
  final BoxFit fit;
  final ProleadImageTransform? transform;
  final Widget Function(BuildContext, Widget, ImageChunkEvent?)? loadingBuilder;
  final Widget Function(BuildContext, Object, StackTrace?)? errorBuilder;

  const ProleadFileImage({
    super.key,
    required this.client,
    required this.rawDownloadUrl,
    this.lqip,
    this.width,
    this.height,
    this.fit = BoxFit.cover,
    this.transform,
    this.loadingBuilder,
    this.errorBuilder,
  });

  Uint8List? _decodeLQIP(String? rawLqip) {
    if (rawLqip == null || rawLqip.isEmpty) return null;
    try {
      final cleanBase64 = rawLqip.contains(',') ? rawLqip.split(',').last : rawLqip;
      return base64Decode(cleanBase64);
    } catch (_) {
      return null;
    }
  }

  @override
  Widget build(BuildContext context) {
    final opts = transform ?? ProleadImageTransform(
      width: width?.toInt(),
      height: height?.toInt(),
      fit: _boxFitToString(fit),
      format: 'webp',
    );

    final transformedUrl = client.getTransformedImageUrl(rawDownloadUrl, opts);
    final lqipBytes = _decodeLQIP(lqip);

    return Image.network(
      transformedUrl,
      width: width,
      height: height,
      fit: fit,
      headers: client.apiKey != null && client.apiKey!.isNotEmpty
          ? {
              'Authorization': 'Bearer ${client.apiKey}',
              'X-API-Key': client.apiKey!,
            }
          : null,
      loadingBuilder: loadingBuilder ??
          (ctx, child, progress) {
            if (progress == null) return child;
            if (lqipBytes != null) {
              return Stack(
                fit: StackFit.passthrough,
                children: [
                  ImageFiltered(
                    imageFilter: ImageFilter.blur(sigmaX: 8, sigmaY: 8),
                    child: Image.memory(
                      lqipBytes,
                      width: width,
                      height: height,
                      fit: fit,
                    ),
                  ),
                  Center(
                    child: CircularProgressIndicator(
                      value: progress.expectedTotalBytes != null
                          ? progress.cumulativeBytesLoaded / progress.expectedTotalBytes!
                          : null,
                      strokeWidth: 2,
                    ),
                  ),
                ],
              );
            }
            return SizedBox(
              width: width,
              height: height,
              child: Center(
                child: CircularProgressIndicator(
                  value: progress.expectedTotalBytes != null
                      ? progress.cumulativeBytesLoaded / progress.expectedTotalBytes!
                      : null,
                ),
              ),
            );
          },
      errorBuilder: errorBuilder ??
          (ctx, err, stack) {
            return Container(
              width: width,
              height: height,
              color: Colors.grey.shade900,
              child: const Icon(Icons.broken_image, color: Colors.grey),
            );
          },
    );
  }

  static String _boxFitToString(BoxFit fit) {
    switch (fit) {
      case BoxFit.cover:
        return 'cover';
      case BoxFit.contain:
        return 'contain';
      case BoxFit.fill:
        return 'fill';
      case BoxFit.scaleDown:
      case BoxFit.fitWidth:
      case BoxFit.fitHeight:
      case BoxFit.none:
      default:
        return 'scale';
    }
  }
}

