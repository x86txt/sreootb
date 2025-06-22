"use client";

import * as React from "react";
import { X, ChevronDown } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";

interface MultiSelectOption {
  value: string;
  label: string;
}

interface MultiSelectProps {
  options: MultiSelectOption[];
  selected: string[];
  onChange: (selected: string[]) => void;
  placeholder?: string;
  className?: string;
}

export function MultiSelect({
  options,
  selected,
  onChange,
  placeholder = "Select items...",
  className,
}: MultiSelectProps) {
  const [open, setOpen] = React.useState(false);

  const handleSelect = (value: string) => {
    console.log('Multi-select click detected:', value, 'currently selected:', selected);
    if (selected.includes(value)) {
      onChange(selected.filter((item) => item !== value));
    } else {
      onChange([...selected, value]);
    }
  };

  const handleRemove = (value: string) => {
    onChange(selected.filter((item) => item !== value));
  };

  const handleSelectAll = () => {
    // Filter out the "all" option and get only individual resource options
    const individualOptions = options.filter(option => option.value !== 'all');
    const individualValues = individualOptions.map(option => option.value);
    
    console.log('Select All clicked:', { 
      individualValues, 
      currentSelected: selected,
      individualOptions: individualOptions.map(o => ({value: o.value, label: o.label}))
    });
    
    // Check if all individual resources are currently selected (excluding 'all')
    const currentIndividualSelected = selected.filter(s => s !== 'all');
    const allIndividualSelected = individualValues.length > 0 && 
                                 individualValues.every(value => currentIndividualSelected.includes(value)) &&
                                 currentIndividualSelected.length === individualValues.length;
    
    console.log('Select All state:', { 
      currentIndividualSelected, 
      allIndividualSelected,
      willSelect: !allIndividualSelected ? individualValues : []
    });
    
    if (allIndividualSelected) {
      // All individual resources are selected, so deselect all
      console.log('Deselecting all');
      onChange([]);
    } else {
      // Select all individual resources at once
      console.log('Selecting all individual resources:', individualValues);
      onChange(individualValues);
    }
  };

  return (
    <div className={className}>
      <div className="relative">
        <Button
          variant="outline"
          className="w-full h-auto min-h-10 justify-between"
          onClick={() => setOpen(!open)}
        >
          <div className="flex flex-wrap gap-1 flex-1">
            {selected.length > 0 ? (
              selected.map((value) => {
                const option = options.find((opt) => opt.value === value);
                return (
                  <Badge key={value} variant="secondary" className="text-xs">
                    {option?.label}
                    <X
                      className="ml-1 h-3 w-3 cursor-pointer"
                      onClick={(e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        handleRemove(value);
                      }}
                    />
                  </Badge>
                );
              })
            ) : (
              <span className="text-muted-foreground">{placeholder}</span>
            )}
          </div>
          <ChevronDown className="h-4 w-4 opacity-50" />
        </Button>

        {open && (
          <Card className="absolute top-full left-0 right-0 z-50 mt-1">
            <CardContent className="p-0">
              <div className="max-h-72 overflow-y-auto">
                <div className="p-2 border-b">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="w-full justify-start"
                    onMouseDown={e => e.preventDefault()}
                    onClick={e => {
                      e.preventDefault();
                      e.stopPropagation();
                      handleSelectAll();
                    }}
                  >
                    {(() => {
                      const individualOptions = options.filter(option => option.value !== 'all');
                      const individualValues = individualOptions.map(option => option.value);
                      const currentIndividualSelected = selected.filter(s => s !== 'all');
                      const allIndividualSelected = individualValues.length > 0 && 
                                                  individualValues.every(value => currentIndividualSelected.includes(value)) &&
                                                  currentIndividualSelected.length === individualValues.length;
                      return allIndividualSelected ? "Deselect All" : "Select All";
                    })()}
                  </Button>
                </div>
                {options.map((option) => (
                  <div
                    key={option.value}
                    className="flex items-center space-x-2 p-2 hover:bg-accent cursor-pointer"
                    onClick={() => handleSelect(option.value)}
                  >
                    <div
                      className={`h-4 w-4 border rounded-sm ${
                        selected.includes(option.value)
                          ? "bg-primary border-primary"
                          : "border-muted-foreground"
                      }`}
                    >
                      {selected.includes(option.value) && (
                        <div className="h-2 w-2 bg-primary-foreground rounded-sm m-0.5" />
                      )}
                    </div>
                    <span>{option.label}</span>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        )}
      </div>

      {/* Overlay to close dropdown when clicking outside */}
      {open && (
        <div 
          className="fixed inset-0 z-40" 
          onClick={() => setOpen(false)}
        />
      )}
    </div>
  );
} 