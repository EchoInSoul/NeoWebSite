"use client";

import { useState, useSyncExternalStore, type ReactNode } from "react";
import { Bar, BarChart, CartesianGrid, XAxis } from "recharts";

import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import {
  Attachment,
  AttachmentAction,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentMedia,
  AttachmentTitle,
} from "@/components/ui/attachment";
import {
  Avatar,
  AvatarBadge,
  AvatarFallback,
  AvatarGroup,
  AvatarGroupCount,
  AvatarImage,
} from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import {
  Breadcrumb,
  BreadcrumbEllipsis,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
} from "@/components/ui/carousel";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from "@/components/ui/command";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuShortcut,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSeparator,
  FieldSet,
  FieldTitle,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
  InputGroupText,
} from "@/components/ui/input-group";
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSeparator,
  InputOTPSlot,
} from "@/components/ui/input-otp";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemFooter,
  ItemGroup,
  ItemHeader,
  ItemMedia,
  ItemSeparator,
  ItemTitle,
} from "@/components/ui/item";
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import { Label } from "@/components/ui/label";
import { Marker, MarkerContent, MarkerIcon } from "@/components/ui/marker";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  NavigationMenu,
  NavigationMenuContent,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
  NavigationMenuTrigger,
} from "@/components/ui/navigation-menu";
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Progress,
  ProgressIndicator,
  ProgressLabel,
  ProgressTrack,
  ProgressValue,
} from "@/components/ui/progress";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from "@/components/ui/sidebar";
import { Skeleton } from "@/components/ui/skeleton";
import { Slider } from "@/components/ui/slider";
import { Toaster as SonnerToaster } from "@/components/ui/sonner";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { Toaster as BaseToaster, toast } from "@/components/ui/toast";
import { Toggle } from "@/components/ui/toggle";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { toast as sonnerToast } from "sonner";
import {
  AlignCenterIcon,
  AlignLeftIcon,
  BoldIcon,
  CheckIcon,
  CircleAlertIcon,
  FileTextIcon,
  InfoIcon,
  ItalicIcon,
  PaperclipIcon,
  PlusIcon,
  TrashIcon,
} from "lucide-react";

const AVATAR_DATA_URI =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" fill="#0a0a0a"/><circle cx="32" cy="24" r="11" fill="#fafafa"/><ellipse cx="32" cy="54" rx="19" ry="14" fill="#fafafa"/></svg>',
  );

function useIsHydrated() {
  return useSyncExternalStore(
    () => () => {},
    () => true,
    () => false,
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-5">
      <h2 className="text-xl font-semibold tracking-tight">{title}</h2>
      <div className="flex flex-wrap items-start gap-x-8 gap-y-6">
        {children}
      </div>
    </section>
  );
}

function Demo({
  label,
  children,
  className,
  wide = false,
}: {
  label: string;
  children: ReactNode;
  className?: string;
  wide?: boolean;
}) {
  return (
    <div
      className={
        className ??
        (wide ? "flex w-full flex-col gap-2" : "flex w-fit flex-col gap-2")
      }
    >
      <span className="font-mono text-xs text-muted-foreground">{label}</span>
      <div>{children}</div>
    </div>
  );
}

const chartData = [
  { month: "一月", desktop: 186, mobile: 80 },
  { month: "二月", desktop: 305, mobile: 200 },
  { month: "三月", desktop: 237, mobile: 120 },
  { month: "四月", desktop: 173, mobile: 190 },
];

const chartConfig = {
  desktop: { label: "桌面端", color: "var(--chart-2)" },
  mobile: { label: "移动端", color: "var(--chart-4)" },
} satisfies ChartConfig;

const demoItems = ["按钮", "卡片", "对话框", "表格", "图表"];

export default function UiCheckPage() {
  const isHydrated = useIsHydrated();
  const [checked, setChecked] = useState(true);
  const [radio, setRadio] = useState("a");
  const [switched, setSwitched] = useState(true);
  const [sliderValue, setSliderValue] = useState<number[]>([40]);
  const [otp, setOtp] = useState("12345");
  const [date, setDate] = useState<Date | undefined>(undefined);

  return (
    <main className="mx-auto flex w-full max-w-5xl flex-col gap-10 p-8 pb-24">
      <header className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">
          shadcn/ui 全组件检查页
        </h1>
        <p className="text-sm text-muted-foreground">
          共 47 个组件（base 底座 / nova 预设）。
          <code className="font-mono">form</code> 在 Base UI 注册表里是空壳，由{" "}
          <code className="font-mono">field</code> 取代。
        </p>
      </header>

      <Separator />

      <Section title="按钮与视觉标记">
        <Demo label="button">
          <div className="flex flex-wrap items-center gap-2">
            <Button>默认</Button>
            <Button variant="secondary">次要</Button>
            <Button variant="outline">描边</Button>
            <Button variant="ghost">幽灵</Button>
            <Button variant="destructive">危险</Button>
            <Button variant="link">链接</Button>
          </div>
        </Demo>
        <Demo label="button 尺寸 / 图标">
          <div className="flex flex-wrap items-center gap-2">
            <Button size="xs">xs</Button>
            <Button size="sm">sm</Button>
            <Button size="default">default</Button>
            <Button size="lg">lg</Button>
            <Button size="icon">
              <PlusIcon />
            </Button>
            <Button disabled>
              <Spinner /> 处理中
            </Button>
          </div>
        </Demo>
        <Demo label="badge">
          <div className="flex flex-wrap items-center gap-2">
            <Badge>默认</Badge>
            <Badge variant="secondary">次要</Badge>
            <Badge variant="outline">描边</Badge>
            <Badge variant="destructive">危险</Badge>
          </div>
        </Demo>
        <Demo label="kbd">
          <KbdGroup>
            <Kbd>Ctrl</Kbd>
            <Kbd>Shift</Kbd>
            <Kbd>K</Kbd>
          </KbdGroup>
        </Demo>
        <Demo label="separator">
          <div className="flex h-10 items-center gap-3">
            <span className="text-sm">上</span>
            <Separator orientation="vertical" />
            <span className="text-sm">下</span>
          </div>
        </Demo>
        <Demo label="skeleton">
          <div className="flex flex-col gap-2">
            <Skeleton className="h-4 w-48" />
            <Skeleton className="h-4 w-32" />
          </div>
        </Demo>
        <Demo label="spinner">
          <div className="flex items-center gap-3">
            <Spinner />
            <Spinner className="size-6" />
          </div>
        </Demo>
        <Demo label="marker">
          <div className="flex flex-col gap-2">
            <Marker>
              <MarkerIcon>
                <InfoIcon />
              </MarkerIcon>
              <MarkerContent>默认标记</MarkerContent>
            </Marker>
            <Marker variant="separator">
              <MarkerContent>分隔标记</MarkerContent>
            </Marker>
          </div>
        </Demo>
        <Demo label="toggle">
          <div className="flex items-center gap-2">
            <Toggle aria-label="加粗">
              <BoldIcon />
            </Toggle>
            <Toggle variant="outline" aria-label="斜体">
              <ItalicIcon />
            </Toggle>
            <Toggle size="sm">
              <AlignLeftIcon /> 左对齐
            </Toggle>
          </div>
        </Demo>
        <Demo label="toggle-group">
          <ToggleGroup defaultValue={["center"]}>
            <ToggleGroupItem value="left" aria-label="左">
              <AlignLeftIcon />
            </ToggleGroupItem>
            <ToggleGroupItem value="center" aria-label="中">
              <AlignCenterIcon />
            </ToggleGroupItem>
          </ToggleGroup>
        </Demo>
      </Section>

      <Separator />

      <Section title="输入与表单">
        <Demo label="input">
          <Input placeholder="请输入标题" className="w-64" />
        </Demo>
        <Demo label="textarea">
          <Textarea placeholder="请输入正文" className="w-64" />
        </Demo>
        <Demo label="label">
          <Label htmlFor="demo-label">文章标题</Label>
        </Demo>
        <Demo label="checkbox">
          <div className="flex items-center gap-2">
            <Checkbox
              id="demo-checkbox"
              checked={checked}
              onCheckedChange={setChecked}
            />
            <Label htmlFor="demo-checkbox">接收评论通知</Label>
          </div>
        </Demo>
        <Demo label="radio-group">
          <RadioGroup
            value={radio}
            onValueChange={setRadio}
            className="flex flex-col gap-2"
          >
            <div className="flex items-center gap-2">
              <RadioGroupItem value="a" id="radio-a" />
              <Label htmlFor="radio-a">公开</Label>
            </div>
            <div className="flex items-center gap-2">
              <RadioGroupItem value="b" id="radio-b" />
              <Label htmlFor="radio-b">私密</Label>
            </div>
          </RadioGroup>
        </Demo>
        <Demo label="switch">
          <div className="flex items-center gap-2">
            <Switch
              id="demo-switch"
              checked={switched}
              onCheckedChange={setSwitched}
            />
            <Label htmlFor="demo-switch">开启评论</Label>
          </div>
        </Demo>
        <Demo label="slider">
          <div className="w-56">
            <Slider
              value={sliderValue}
              onValueChange={(value) =>
                setSliderValue(
                  Array.isArray(value) ? [...value] : [value as number],
                )
              }
            />
            <p className="mt-2 text-xs text-muted-foreground">
              当前值：{sliderValue[0]}
            </p>
          </div>
        </Demo>
        <Demo label="select">
          <Select defaultValue="apple">
            <SelectTrigger className="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="apple">苹果</SelectItem>
              <SelectItem value="banana">香蕉</SelectItem>
              <SelectItem value="grape">葡萄</SelectItem>
            </SelectContent>
          </Select>
        </Demo>
        <Demo label="native-select">
          <NativeSelect defaultValue="apple">
            <NativeSelectOption value="apple">苹果</NativeSelectOption>
            <NativeSelectOption value="banana">香蕉</NativeSelectOption>
          </NativeSelect>
        </Demo>
        <Demo label="input-otp">
          <InputOTP maxLength={6} value={otp} onChange={setOtp}>
            <InputOTPGroup>
              <InputOTPSlot index={0} />
              <InputOTPSlot index={1} />
              <InputOTPSlot index={2} />
            </InputOTPGroup>
            <InputOTPSeparator />
            <InputOTPGroup>
              <InputOTPSlot index={3} />
              <InputOTPSlot index={4} />
              <InputOTPSlot index={5} />
            </InputOTPGroup>
          </InputOTP>
        </Demo>
        <Demo label="input-group">
          <InputGroup className="w-64">
            <InputGroupAddon align="inline-start">
              <InputGroupText>https://</InputGroupText>
            </InputGroupAddon>
            <InputGroupInput placeholder="example.com" />
            <InputGroupAddon align="inline-end">
              <InputGroupButton>
                <CheckIcon />
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
        </Demo>
        <Demo label="field">
          <FieldSet className="w-72">
            <FieldLegend>发布设置</FieldLegend>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="field-slug">别名</FieldLabel>
                <FieldContent>
                  <Input id="field-slug" placeholder="my-first-post" />
                  <FieldDescription>
                    用于访问地址，只能包含小写字母和短横线。
                  </FieldDescription>
                </FieldContent>
              </Field>
              <FieldSeparator />
              <Field orientation="horizontal">
                <FieldTitle>置顶</FieldTitle>
                <Switch />
              </Field>
              <Field data-invalid="true">
                <FieldLabel htmlFor="field-bad">错误示例</FieldLabel>
                <Input id="field-bad" aria-invalid defaultValue="非法 值" />
                <FieldError>只能包含小写字母和短横线</FieldError>
              </Field>
            </FieldGroup>
          </FieldSet>
        </Demo>
        <Demo label="attachment">
          <div className="flex w-80 flex-col gap-2">
            <Attachment>
              <AttachmentMedia>
                <FileTextIcon />
              </AttachmentMedia>
              <AttachmentContent>
                <AttachmentTitle>设计稿.pdf</AttachmentTitle>
                <AttachmentDescription>2.4 MB</AttachmentDescription>
              </AttachmentContent>
              <AttachmentActions>
                <AttachmentAction aria-label="删除">
                  <TrashIcon />
                </AttachmentAction>
              </AttachmentActions>
            </Attachment>
            <Attachment state="uploading" size="sm">
              <AttachmentMedia>
                <Spinner />
              </AttachmentMedia>
              <AttachmentContent>
                <AttachmentTitle>封面.png</AttachmentTitle>
              </AttachmentContent>
            </Attachment>
          </div>
        </Demo>
        <Demo label="calendar" className="flex w-fit flex-col gap-2">
          {isHydrated ? (
            <Calendar mode="single" selected={date} onSelect={setDate} />
          ) : (
            <Skeleton className="h-72 w-70" />
          )}
        </Demo>
        <Demo label="combobox">
          <Combobox items={demoItems} defaultValue="卡片">
            <ComboboxInput placeholder="搜索组件" className="w-56" />
            <ComboboxContent>
              <ComboboxEmpty>没有匹配项</ComboboxEmpty>
              <ComboboxList>
                {(item: string) => (
                  <ComboboxItem key={item} value={item}>
                    {item}
                  </ComboboxItem>
                )}
              </ComboboxList>
            </ComboboxContent>
          </Combobox>
        </Demo>
      </Section>

      <Separator />

      <Section title="布局与容器">
        <Demo label="card">
          <Card className="w-72">
            <CardHeader>
              <CardTitle>卡片标题</CardTitle>
              <CardDescription>卡片描述文字</CardDescription>
              <CardAction>
                <Badge variant="secondary">新</Badge>
              </CardAction>
            </CardHeader>
            <CardContent className="text-sm">卡片主体内容。</CardContent>
            <CardFooter>
              <Button size="sm" variant="outline">
                查看
              </Button>
            </CardFooter>
          </Card>
        </Demo>
        <Demo label="item">
          <ItemGroup className="w-80">
            <Item>
              <ItemMedia variant="icon">
                <FileTextIcon />
              </ItemMedia>
              <ItemContent>
                <ItemTitle>第一篇文章</ItemTitle>
                <ItemDescription>2026-10-11 · 3 分钟</ItemDescription>
              </ItemContent>
              <ItemActions>
                <Button size="icon-sm" variant="ghost" aria-label="删除">
                  <TrashIcon />
                </Button>
              </ItemActions>
            </Item>
            <ItemSeparator />
            <Item variant="outline">
              <ItemHeader>
                <ItemTitle>带页眉的分组</ItemTitle>
              </ItemHeader>
              <ItemContent>
                <ItemDescription>outline 变体</ItemDescription>
              </ItemContent>
              <ItemFooter>
                <span className="text-xs text-muted-foreground">页脚</span>
              </ItemFooter>
            </Item>
          </ItemGroup>
        </Demo>
        <Demo label="empty">
          <Empty className="w-80">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <FileTextIcon />
              </EmptyMedia>
              <EmptyTitle>还没有文章</EmptyTitle>
              <EmptyDescription>
                写第一篇草稿之后，它会出现在这里。
              </EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button size="sm">
                <PlusIcon /> 新建文章
              </Button>
            </EmptyContent>
          </Empty>
        </Demo>
        <Demo label="avatar">
          <div className="flex items-center gap-3">
            <Avatar>
              <AvatarImage src={AVATAR_DATA_URI} alt="头像" />
              <AvatarFallback>NB</AvatarFallback>
              <AvatarBadge />
            </Avatar>
            <Avatar>
              <AvatarFallback>EC</AvatarFallback>
            </Avatar>
            <AvatarGroup>
              <Avatar>
                <AvatarFallback>A</AvatarFallback>
              </Avatar>
              <Avatar>
                <AvatarFallback>B</AvatarFallback>
              </Avatar>
              <AvatarGroupCount>+3</AvatarGroupCount>
            </AvatarGroup>
          </div>
        </Demo>
        <Demo label="table" wide>
          <Table>
            <TableCaption>文章列表</TableCaption>
            <TableHeader>
              <TableRow>
                <TableHead>标题</TableHead>
                <TableHead>状态</TableHead>
                <TableHead className="text-right">阅读量</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell className="font-medium">第一篇</TableCell>
                <TableCell>
                  <Badge variant="secondary">草稿</Badge>
                </TableCell>
                <TableCell className="text-right">128</TableCell>
              </TableRow>
              <TableRow>
                <TableCell className="font-medium">第二篇</TableCell>
                <TableCell>
                  <Badge>已发布</Badge>
                </TableCell>
                <TableCell className="text-right">1,204</TableCell>
              </TableRow>
            </TableBody>
            <TableFooter>
              <TableRow>
                <TableCell colSpan={2}>合计</TableCell>
                <TableCell className="text-right">1,332</TableCell>
              </TableRow>
            </TableFooter>
          </Table>
        </Demo>
      </Section>

      <Separator />

      <Section title="导航">
        <Demo label="breadcrumb">
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem>
                <BreadcrumbLink href="#">首页</BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbLink href="#">文章</BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbEllipsis />
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>当前页</BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
        </Demo>
        <Demo label="pagination">
          <Pagination>
            <PaginationContent>
              <PaginationItem>
                <PaginationPrevious href="#" />
              </PaginationItem>
              <PaginationItem>
                <PaginationLink href="#">1</PaginationLink>
              </PaginationItem>
              <PaginationItem>
                <PaginationLink href="#" isActive>
                  2
                </PaginationLink>
              </PaginationItem>
              <PaginationItem>
                <PaginationLink href="#">3</PaginationLink>
              </PaginationItem>
              <PaginationItem>
                <PaginationEllipsis />
              </PaginationItem>
              <PaginationItem>
                <PaginationNext href="#" />
              </PaginationItem>
            </PaginationContent>
          </Pagination>
        </Demo>
        <Demo label="navigation-menu" wide>
          <NavigationMenu>
            <NavigationMenuList>
              <NavigationMenuItem>
                <NavigationMenuTrigger>文章</NavigationMenuTrigger>
                <NavigationMenuContent>
                  <div className="flex w-48 flex-col p-2">
                    <NavigationMenuLink href="#">全部文章</NavigationMenuLink>
                    <NavigationMenuLink href="#">标签</NavigationMenuLink>
                  </div>
                </NavigationMenuContent>
              </NavigationMenuItem>
              <NavigationMenuItem>
                <NavigationMenuLink href="#">关于</NavigationMenuLink>
              </NavigationMenuItem>
            </NavigationMenuList>
          </NavigationMenu>
        </Demo>
        <Demo label="tabs">
          <Tabs defaultValue="one">
            <TabsList>
              <TabsTrigger value="one">标签一</TabsTrigger>
              <TabsTrigger value="two">标签二</TabsTrigger>
              <TabsTrigger value="three">标签三</TabsTrigger>
            </TabsList>
            <TabsContent value="one">第一个面板</TabsContent>
            <TabsContent value="two">第二个面板</TabsContent>
            <TabsContent value="three">第三个面板</TabsContent>
          </Tabs>
        </Demo>
        <Demo label="sidebar" wide>
          <div className="h-72 w-full overflow-hidden rounded-lg border">
            <SidebarProvider>
              <Sidebar collapsible="none" className="h-full">
                <SidebarHeader>
                  <span className="px-2 py-1 text-sm font-medium">NeoBlog</span>
                </SidebarHeader>
                <SidebarContent>
                  <SidebarGroup>
                    <SidebarGroupLabel>内容</SidebarGroupLabel>
                    <SidebarGroupContent>
                      <SidebarMenu>
                        <SidebarMenuItem>
                          <SidebarMenuButton isActive>
                            <FileTextIcon />
                            <span>文章</span>
                          </SidebarMenuButton>
                          <SidebarMenuBadge>12</SidebarMenuBadge>
                        </SidebarMenuItem>
                        <SidebarMenuItem>
                          <SidebarMenuButton>
                            <PaperclipIcon />
                            <span>媒体</span>
                          </SidebarMenuButton>
                        </SidebarMenuItem>
                      </SidebarMenu>
                    </SidebarGroupContent>
                  </SidebarGroup>
                </SidebarContent>
              </Sidebar>
              <SidebarInset className="p-4 text-sm">主内容区</SidebarInset>
            </SidebarProvider>
          </div>
        </Demo>
      </Section>

      <Separator />

      <Section title="浮层与菜单">
        <Demo label="dialog">
          <Dialog>
            <DialogTrigger render={<Button variant="outline" />}>
              打开对话框
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>对话框标题</DialogTitle>
                <DialogDescription>对话框描述文字。</DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button>确认</Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </Demo>
        <Demo label="alert-dialog">
          <AlertDialog>
            <AlertDialogTrigger render={<Button variant="outline" />}>
              删除确认
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogMedia>
                  <CircleAlertIcon />
                </AlertDialogMedia>
                <AlertDialogTitle>确定删除这篇文章？</AlertDialogTitle>
                <AlertDialogDescription>
                  删除后无法恢复。
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>取消</AlertDialogCancel>
                <AlertDialogAction>删除</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </Demo>
        <Demo label="sheet">
          <Sheet>
            <SheetTrigger render={<Button variant="outline" />}>
              打开侧栏
            </SheetTrigger>
            <SheetContent>
              <SheetHeader>
                <SheetTitle>侧栏标题</SheetTitle>
                <SheetDescription>侧栏描述文字。</SheetDescription>
              </SheetHeader>
            </SheetContent>
          </Sheet>
        </Demo>
        <Demo label="popover">
          <Popover>
            <PopoverTrigger render={<Button variant="outline" />}>
              打开浮层
            </PopoverTrigger>
            <PopoverContent>
              <PopoverHeader>
                <PopoverTitle>浮层标题</PopoverTitle>
                <PopoverDescription>浮层描述文字。</PopoverDescription>
              </PopoverHeader>
            </PopoverContent>
          </Popover>
        </Demo>
        <Demo label="tooltip">
          <Tooltip>
            <TooltipTrigger render={<Button variant="ghost" />}>
              悬停看提示
            </TooltipTrigger>
            <TooltipContent>提示内容</TooltipContent>
          </Tooltip>
        </Demo>
        <Demo label="context-menu">
          <ContextMenu>
            <ContextMenuTrigger className="flex h-24 w-56 items-center justify-center rounded-lg border border-dashed text-sm">
              在这里点右键
            </ContextMenuTrigger>
            <ContextMenuContent>
              <ContextMenuLabel>操作</ContextMenuLabel>
              <ContextMenuSeparator />
              <ContextMenuItem>
                复制 <ContextMenuShortcut>⌘C</ContextMenuShortcut>
              </ContextMenuItem>
              <ContextMenuItem variant="destructive">删除</ContextMenuItem>
            </ContextMenuContent>
          </ContextMenu>
        </Demo>
        <Demo label="command">
          <Command className="w-72 rounded-lg border">
            <CommandInput placeholder="输入命令…" />
            <CommandList>
              <CommandEmpty>没有结果</CommandEmpty>
              <CommandGroup heading="建议">
                <CommandItem>
                  打开设置 <CommandShortcut>⌘,</CommandShortcut>
                </CommandItem>
                <CommandItem>新建文章</CommandItem>
              </CommandGroup>
              <CommandSeparator />
              <CommandGroup heading="其它">
                <CommandItem>切换主题</CommandItem>
              </CommandGroup>
            </CommandList>
          </Command>
        </Demo>
      </Section>

      <Separator />

      <Section title="反馈与数据">
        <Demo label="alert" wide>
          <div className="flex w-full flex-col gap-3">
            <Alert>
              <InfoIcon />
              <AlertTitle>默认提示</AlertTitle>
              <AlertDescription>这是一条普通信息。</AlertDescription>
            </Alert>
            <Alert variant="destructive">
              <CircleAlertIcon />
              <AlertTitle>出错了</AlertTitle>
              <AlertDescription>保存失败，请重试。</AlertDescription>
              <AlertAction>
                <Button size="xs" variant="outline">
                  重试
                </Button>
              </AlertAction>
            </Alert>
          </div>
        </Demo>
        <Demo label="progress">
          <div className="w-64">
            <Progress value={62}>
              <ProgressLabel>上传进度</ProgressLabel>
              <ProgressValue />
              <ProgressTrack>
                <ProgressIndicator />
              </ProgressTrack>
            </Progress>
          </div>
        </Demo>
        <Demo label="chart" wide>
          <ChartContainer config={chartConfig} className="h-56 w-full">
            <BarChart accessibilityLayer data={chartData}>
              <CartesianGrid vertical={false} />
              <XAxis dataKey="month" tickLine={false} axisLine={false} />
              <ChartTooltip content={<ChartTooltipContent />} />
              <Bar dataKey="desktop" fill="var(--color-desktop)" radius={4} />
              <Bar dataKey="mobile" fill="var(--color-mobile)" radius={4} />
            </BarChart>
          </ChartContainer>
        </Demo>
        <Demo label="carousel">
          <div className="w-64">
            <Carousel>
              <CarouselContent>
                {Array.from({ length: 5 }, (_, i) => (
                  <CarouselItem key={i}>
                    <div className="flex h-32 items-center justify-center rounded-lg bg-muted text-sm">
                      第 {i + 1} 张
                    </div>
                  </CarouselItem>
                ))}
              </CarouselContent>
              <CarouselPrevious />
              <CarouselNext />
            </Carousel>
          </div>
        </Demo>
        <Demo label="sonner（点击按钮）">
          <Button
            variant="outline"
            onClick={() =>
              sonnerToast.success("已保存", { description: "文章已存入草稿箱" })
            }
          >
            弹出 sonner 消息
          </Button>
        </Demo>
        <Demo label="toast（Base UI，点击按钮）">
          <Button
            variant="outline"
            onClick={() =>
              toast.add({
                title: "已发布",
                description: "文章已发布到线上",
                type: "success",
              })
            }
          >
            弹出 toast 消息
          </Button>
        </Demo>
      </Section>
      <SonnerToaster />
      <BaseToaster />
    </main>
  );
}
