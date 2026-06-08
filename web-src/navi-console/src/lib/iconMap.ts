import {
  Folder, Code, Music, Book, Briefcase, Database, Globe, Heart, Image, Layout,
  Monitor, Package, Server, Settings, Star, Terminal, Wrench, User, Zap,
  Camera, Coffee, Compass, Cpu, Film, Flag, Gift, Home, Key, Layers, Lightbulb,
  Infinity,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

export const ICON_MAP: Record<string, LucideIcon> = {
  folder: Folder, code: Code, music: Music, book: Book, briefcase: Briefcase,
  database: Database, globe: Globe, heart: Heart, image: Image, layout: Layout,
  monitor: Monitor, package: Package, server: Server, settings: Settings, star: Star,
  terminal: Terminal, wrench: Wrench, user: User, zap: Zap, camera: Camera,
  coffee: Coffee, compass: Compass, cpu: Cpu, film: Film, flag: Flag,
  gift: Gift, home: Home, key: Key, layers: Layers, lightbulb: Lightbulb,
  infinity: Infinity,
};

export const PRESET_COLORS = [
  { name: 'Blue',   hex: '#3b82f6' },
  { name: 'Purple', hex: '#a855f7' },
  { name: 'Green',  hex: '#22c55e' },
  { name: 'Yellow', hex: '#eab308' },
  { name: 'Orange', hex: '#f97316' },
  { name: 'Red',    hex: '#ef4444' },
  { name: 'Pink',   hex: '#ec4899' },
  { name: 'White',  hex: '#e2e8f0' },
];
