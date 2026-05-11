import React from 'react';

interface IconProps {
  size?: number;
  color?: string;
  className?: string;
}

export const GridIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <rect x="2" y="2" width="6.5" height="6.5" rx="1.5" stroke={color} strokeWidth="1.5" />
    <rect x="11.5" y="2" width="6.5" height="6.5" rx="1.5" stroke={color} strokeWidth="1.5" />
    <rect x="2" y="11.5" width="6.5" height="6.5" rx="1.5" stroke={color} strokeWidth="1.5" />
    <rect x="11.5" y="11.5" width="6.5" height="6.5" rx="1.5" stroke={color} strokeWidth="1.5" />
  </svg>
);

export const FolderIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M2.5 5.5C2.5 4.395 3.395 3.5 4.5 3.5H8L9.5 5.5H15.5C16.605 5.5 17.5 6.395 17.5 7.5V14.5C17.5 15.605 16.605 16.5 15.5 16.5H4.5C3.395 16.5 2.5 15.605 2.5 14.5V5.5Z" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
  </svg>
);

export const FolderOpenIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M2.5 5.5C2.5 4.395 3.395 3.5 4.5 3.5H8L9.5 5.5H15.5C16.605 5.5 17.5 6.395 17.5 7.5V8H4.656C3.618 8 2.762 8.793 2.663 9.826L2.5 11.5V5.5Z" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
    <path d="M2.5 11.5L4.2 14.8C4.389 15.178 4.775 15.414 5.196 15.414H17.5V9.5C17.5 8.672 16.828 8 16 8H4.656C3.618 8 2.762 8.793 2.663 9.826L2.5 11.5Z" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
  </svg>
);

export const LayersIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M10 3L17.5 7.5L10 12L2.5 7.5L10 3Z" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
    <path d="M2.5 11L10 15.5L17.5 11" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
    <path d="M2.5 14.5L10 19L17.5 14.5" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
  </svg>
);

export const GearIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <circle cx="10" cy="10" r="2.5" stroke={color} strokeWidth="1.5" />
    <path d="M10 2V4M10 16V18M2 10H4M16 10H18M4.22 4.22L5.64 5.64M14.36 14.36L15.78 15.78M15.78 4.22L14.36 5.64M5.64 14.36L4.22 15.78" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

export const WarningIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M10 3L18 17H2L10 3Z" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
    <path d="M10 8.5V11.5" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
    <circle cx="10" cy="13.75" r="0.75" fill={color} />
  </svg>
);

export const ClockIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <circle cx="10" cy="10" r="7.5" stroke={color} strokeWidth="1.5" />
    <path d="M10 6V10.5L13 12.5" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const DocumentIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M5 3.5C5 2.672 5.672 2 6.5 2H12L16 6V16.5C16 17.328 15.328 18 14.5 18H6.5C5.672 18 5 17.328 5 16.5V3.5Z" stroke={color} strokeWidth="1.5" />
    <path d="M12 2V6H16" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
    <path d="M8 10H12M8 13H11" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

export const SyncIcon: React.FC<IconProps & { spinning?: boolean }> = ({ size = 20, color = 'currentColor', className, spinning }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={spinning ? 'spin' : className}>
    <path d="M4 10C4 6.686 6.686 4 10 4C12.21 4 14.117 5.24 15.118 7H13" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
    <path d="M16 10C16 13.314 13.314 16 10 16C7.79 16 5.883 14.76 4.882 13H7" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

export const CloudIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M5.5 14.5C3.29 14.5 1.5 12.71 1.5 10.5C1.5 8.29 3.29 6.5 5.5 6.5C5.67 6.5 5.837 6.51 6 6.527C6.467 4.117 8.606 2.3 11.15 2.3C14.046 2.3 16.4 4.654 16.4 7.55C16.4 7.768 16.387 7.983 16.36 8.193C17.625 8.854 18.5 10.178 18.5 11.7C18.5 13.909 16.71 15.7 14.5 15.7H5.5V14.5Z" stroke={color} strokeWidth="1.5" />
  </svg>
);

export const CheckIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M4 10.5L8 14.5L16 5.5" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const CloseIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M5.5 5.5L14.5 14.5M14.5 5.5L5.5 14.5" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

export const DashIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M5 10H15" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

export const ChevronRightIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M7.5 5L12.5 10L7.5 15" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const DotsIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <circle cx="4" cy="10" r="1.5" fill={color} />
    <circle cx="10" cy="10" r="1.5" fill={color} />
    <circle cx="16" cy="10" r="1.5" fill={color} />
  </svg>
);

export const UploadIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M10 14V3M10 3L6.5 6.5M10 3L13.5 6.5" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    <path d="M3.5 14V15.5C3.5 16.605 4.395 17.5 5.5 17.5H14.5C15.605 17.5 16.5 16.605 16.5 15.5V14" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

export const DownloadIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M10 3V14M10 14L6.5 10.5M10 14L13.5 10.5" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    <path d="M3.5 14V15.5C3.5 16.605 4.395 17.5 5.5 17.5H14.5C15.605 17.5 16.5 16.605 16.5 15.5V14" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

export const FileIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M5.5 2.5H12L16.5 7V16.5C16.5 17.052 16.052 17.5 15.5 17.5H5.5C4.948 17.5 4.5 17.052 4.5 16.5V3.5C4.5 2.948 4.948 2.5 5.5 2.5Z" stroke={color} strokeWidth="1.5" />
    <path d="M12 2.5V7H16.5" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
  </svg>
);

export const PlayIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M5.5 3.5L16.5 10L5.5 16.5V3.5Z" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
  </svg>
);

export const PauseIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <rect x="5" y="3.5" width="3.5" height="13" rx="1" stroke={color} strokeWidth="1.5" />
    <rect x="11.5" y="3.5" width="3.5" height="13" rx="1" stroke={color} strokeWidth="1.5" />
  </svg>
);

export const MoonIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <path d="M17.5 11.5C16.26 12.74 14.53 13.5 12.63 13.5C8.83 13.5 5.75 10.42 5.75 6.62C5.75 4.72 6.51 2.99 7.75 1.75C4.31 2.78 1.75 5.98 1.75 9.75C1.75 14.31 5.44 18 10 18C13.77 18 16.97 15.44 18 12C17.84 11.84 17.67 11.67 17.5 11.5Z" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
  </svg>
);

export const SunIcon: React.FC<IconProps> = ({ size = 20, color = 'currentColor', className }) => (
  <svg width={size} height={size} viewBox="0 0 20 20" fill="none" className={className}>
    <circle cx="10" cy="10" r="3.5" stroke={color} strokeWidth="1.5" />
    <path d="M10 2V4.5M10 15.5V18M2 10H4.5M15.5 10H18M4.57 4.57L6.34 6.34M13.66 13.66L15.43 15.43M15.43 4.57L13.66 6.34M6.34 13.66L4.57 15.43" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);
