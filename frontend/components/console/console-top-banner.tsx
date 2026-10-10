export function ConsoleTopBanner() {
  return (
    <header className="flex h-(--console-banner-h) w-full shrink-0 items-stretch border-b border-border bg-background">
      <div
        aria-hidden="true"
        className="hidden w-(--sidebar-width) shrink-0 border-r border-border md:block"
      />
      <div className="flex min-w-0 flex-1 items-center px-4 md:px-6" />
    </header>
  );
}
