import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";

export default function proxy(request: NextRequest) {
  if (request.cookies.get("session")?.value) {
    return NextResponse.next();
  }

  return NextResponse.redirect(new URL("/signin", request.nextUrl));
}

export const config = {
  matcher: ["/console/:path*"],
};
