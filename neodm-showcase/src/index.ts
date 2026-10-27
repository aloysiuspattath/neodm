// src/index.ts

export interface Env {
  ASSETS: Fetcher;
  GITHUB_REPO?: string;
}

interface GitHubReleaseAsset {
  name: string;
  browser_download_url: string;
  size: number;
  download_count: number;
}

interface GitHubReleaseResponse {
  tag_name: string;
  name: string;
  html_url: string;
  published_at: string;
  assets: GitHubReleaseAsset[];
}

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const url = new URL(request.url);
    const repo = env.GITHUB_REPO || "aloysiuspattath/neodm";

    // 1. Dynamic 1-Click Installer Direct Download: /download/installer or /download
    if (url.pathname === "/download" || url.pathname === "/download/installer") {
      try {
        const release = await fetchLatestRelease(repo);
        const installer = release.assets.find(
          (a) => a.name.endsWith(".exe") && a.name.includes("Setup")
        );
        const targetUrl = installer?.browser_download_url || `https://github.com/${repo}/releases/latest`;
        return Response.redirect(targetUrl, 302);
      } catch (err) {
        return Response.redirect(`https://github.com/${repo}/releases/latest`, 302);
      }
    }

    // 2. Dynamic Portable Zip Direct Download: /download/portable
    if (url.pathname === "/download/portable") {
      try {
        const release = await fetchLatestRelease(repo);
        const portable = release.assets.find((a) => a.name.endsWith(".zip"));
        const targetUrl = portable?.browser_download_url || `https://github.com/${repo}/releases/latest`;
        return Response.redirect(targetUrl, 302);
      } catch (err) {
        return Response.redirect(`https://github.com/${repo}/releases/latest`, 302);
      }
    }

    // 3. API endpoint for live version badge and metadata: /api/release
    if (url.pathname === "/api/release") {
      try {
        const release = await fetchLatestRelease(repo);
        return Response.json(
          {
            version: release.tag_name,
            name: release.name,
            publishedAt: release.published_at,
            repoUrl: `https://github.com/${repo}`,
            assets: release.assets.map((a) => ({
              name: a.name,
              sizeBytes: a.size,
              sizeMB: (a.size / (1024 * 1024)).toFixed(1) + " MB",
              downloadCount: a.download_count,
              url: a.browser_download_url,
            })),
          },
          {
            headers: {
              "Access-Control-Allow-Origin": "*",
              "Cache-Control": "public, max-age=300, s-maxage=600",
            },
          }
        );
      } catch (err) {
        return Response.json(
          { error: "Failed to fetch GitHub release metadata", details: String(err) },
          { status: 502 }
        );
      }
    }

    // 4. Well-Known LLM Discovery Standard: /.well-known/llms.txt -> /llms.txt
    if (url.pathname === "/.well-known/llms.txt") {
      return Response.redirect(`${url.origin}/llms.txt`, 301);
    }

    // 5. Default: Serve Bento Landing Page & Static Assets from /public
    const response = await env.ASSETS.fetch(request);

    // Apply high-security and SEO edge headers
    const newHeaders = new Headers(response.headers);
    newHeaders.set("X-Content-Type-Options", "nosniff");
    newHeaders.set("X-Frame-Options", "DENY");
    newHeaders.set("Referrer-Policy", "strict-origin-when-cross-origin");

    // Cache-Control headers optimized for Core Web Vitals, Googlebot, and LLM crawlers
    if (url.pathname === "/robots.txt" || url.pathname === "/llms.txt" || url.pathname === "/llms-full.txt") {
      newHeaders.set("Content-Type", "text/plain; charset=utf-8");
      newHeaders.set("Cache-Control", "public, max-age=86400");
    } else if (url.pathname === "/sitemap.xml") {
      newHeaders.set("Content-Type", "application/xml; charset=utf-8");
      newHeaders.set("Cache-Control", "public, max-age=86400");
    } else if (url.pathname.match(/\.(png|jpg|jpeg|webp|ico|svg|woff2)$/)) {
      newHeaders.set("Cache-Control", "public, max-age=604800, stale-while-revalidate=86400");
    } else if (url.pathname.endsWith(".css") || url.pathname.endsWith(".js")) {
      newHeaders.set("Cache-Control", "public, max-age=3600, stale-while-revalidate=600");
    } else {
      newHeaders.set("Cache-Control", "public, max-age=300, stale-while-revalidate=60");
    }

    return new Response(response.body, {
      status: response.status,
      statusText: response.statusText,
      headers: newHeaders,
    });
  },
} satisfies ExportedHandler<Env>;

/**
 * Helper to fetch and edge-cache the latest GitHub release metadata.
 */
async function fetchLatestRelease(repo: string): Promise<GitHubReleaseResponse> {
  const apiUrl = `https://api.github.com/repos/${repo}/releases/latest`;
  const response = await fetch(apiUrl, {
    headers: {
      "User-Agent": "NeoDM-Showcase-Cloudflare-Worker",
      Accept: "application/vnd.github.v3+json",
    },
    cf: {
      cacheTtl: 600, // Edge cache GitHub API responses for 10 minutes
      cacheEverything: true,
    },
  });

  if (!response.ok) {
    throw new Error(`GitHub API returned status ${response.status}`);
  }

  return (await response.json()) as GitHubReleaseResponse;
}
