/**
 * Browser-specific utility helpers for downloading files, creating blob URLs, and formatting.
 */

/**
 * Triggers a native browser file download from a Blob or Uint8Array.
 */
export function downloadBlob(data: Blob | Uint8Array, filename: string, mimeType: string = 'application/octet-stream'): void {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    throw new Error('downloadBlob is only supported in browser environments');
  }

  const blob = data instanceof Blob ? data : new Blob([data.buffer as ArrayBuffer], { type: mimeType });
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.style.display = 'none';
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();

  setTimeout(() => {
    document.body.removeChild(a);
    window.URL.revokeObjectURL(url);
  }, 100);
}

/**
 * Reads a browser File or Blob object into a Uint8Array.
 */
export async function fileToUint8Array(file: Blob): Promise<Uint8Array> {
  const buffer = await file.arrayBuffer();
  return new Uint8Array(buffer);
}

/**
 * Human-readable byte size formatter.
 */
export function formatBytes(bytes: number, decimals: number = 1): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`;
}

/**
 * Derives common MIME type from a file path or extension.
 */
export function getMimeTypeFromName(filename: string): string {
  const ext = filename.split('.').pop()?.toLowerCase();
  switch (ext) {
    case 'jpg':
    case 'jpeg':
      return 'image/jpeg';
    case 'png':
      return 'image/png';
    case 'gif':
      return 'image/gif';
    case 'webp':
      return 'image/webp';
    case 'svg':
      return 'image/svg+xml';
    case 'pdf':
      return 'application/pdf';
    case 'json':
      return 'application/json';
    case 'txt':
    case 'md':
      return 'text/plain; charset=utf-8';
    case 'mp3':
      return 'audio/mpeg';
    case 'wav':
      return 'audio/wav';
    case 'mp4':
      return 'video/mp4';
    case 'webm':
      return 'video/webm';
    case 'zip':
      return 'application/zip';
    default:
      return 'application/octet-stream';
  }
}

export interface DroppedFileEntry {
  file: File;
  relativePath: string;
}

/**
 * Recursively extracts all files with relative paths from a DataTransferItemList containing folders.
 */
export async function traverseDroppedDirectory(items: DataTransferItemList): Promise<DroppedFileEntry[]> {
  const entries: DroppedFileEntry[] = [];

  const readEntry = async (item: any, currentPath: string = '') => {
    if (!item) return;

    if (item.isFile) {
      const file: File = await new Promise((resolve, reject) => {
        item.file(resolve, reject);
      });
      entries.push({
        file,
        relativePath: currentPath ? `${currentPath}/${file.name}` : file.name,
      });
    } else if (item.isDirectory) {
      const dirReader = item.createReader();
      const readEntries = async (): Promise<any[]> => {
        return new Promise((resolve, reject) => {
          dirReader.readEntries(resolve, reject);
        });
      };

      let children: any[] = [];
      do {
        children = await readEntries();
        for (const child of children) {
          const nextPath = currentPath ? `${currentPath}/${item.name}` : item.name;
          await readEntry(child, nextPath);
        }
      } while (children.length > 0);
    }
  };

  for (let i = 0; i < items.length; i++) {
    const item = items[i] as any;
    const entry = typeof item.webkitGetAsEntry === 'function' ? item.webkitGetAsEntry() : null;
    if (entry) {
      await readEntry(entry);
    } else {
      const file = items[i].getAsFile();
      if (file) {
        entries.push({ file, relativePath: file.name });
      }
    }
  }

  return entries;
}

