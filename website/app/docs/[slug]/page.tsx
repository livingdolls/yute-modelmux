import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { docPages } from "../docs-content";
import { DocsShell } from "../docs-shell";

export const dynamicParams = false;

export function generateStaticParams() {
  return Object.keys(docPages)
    .filter((slug) => slug !== "overview")
    .map((slug) => ({ slug }));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata> {
  const { slug } = await params;
  const page = docPages[slug];

  if (!page) {
    return { title: "Documentation" };
  }

  return {
    title: page.title,
    description: page.description,
  };
}

export default async function DocumentationPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const page = docPages[slug];

  if (!page || slug === "overview") {
    notFound();
  }

  return <DocsShell page={page} />;
}
