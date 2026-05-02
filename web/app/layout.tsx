// web/app/layout.tsx
import "./globals.css";
import { Nav } from "@/components/Nav";

export const metadata = {
  title: "art-web",
  description: "A gallery for image-forward work.",
};

export default async function Root({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <Nav />
        {children}
      </body>
    </html>
  );
}
